// Package mediaservice implementa a Media API (ADR-0002): valida o pedido,
// guarda o original, publica o job na fila e responde 202. Não usa FFmpeg nem
// yt-dlp.
package mediaservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/handler"
	"radio-backend/internal/jobs"
	"radio-backend/internal/media"
	"radio-backend/internal/metrics"
	"radio-backend/internal/queue"
	"radio-backend/internal/security"
	"radio-backend/internal/storage"
	"radio-backend/internal/tracing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const (
	// DefaultMaxUpload é o mesmo limite do gateway (ADR-0005).
	DefaultMaxUpload = 100 << 20
	// uploadMemory: acima disso o multipart vai para um arquivo temporário.
	uploadMemory = 1 << 20
	// OriginalsPrefix é onde os uploads ficam até serem convertidos.
	OriginalsPrefix = "originals/"
)

type Config struct {
	InstanceID     string
	Repo           storage.Repository
	Objects        storage.ObjectStore
	Queue          queue.Publisher
	Jobs           jobs.Store
	AllowedOrigins []string
	MaxUpload      int64
	// QueueConnected informa o estado da fila no /healthz (opcional).
	QueueConnected func() bool
}

type Service struct {
	cfg Config
}

func NewService(cfg Config) *Service {
	if cfg.MaxUpload <= 0 {
		cfg.MaxUpload = DefaultMaxUpload
	}
	return &Service{cfg: cfg}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	// Antes do Recoverer: um panic recuperado como 500 entra na taxa de erro.
	r.Use(tracing.HTTPMiddleware("radiosync-media"))
	r.Use(metrics.HTTPMiddleware("media"))
	r.Use(chimw.Recoverer)
	r.Use(security.CORS(s.cfg.AllowedOrigins))

	r.Get("/healthz", s.Healthz)
	r.Handle("/metrics", metrics.Handler())

	r.Route("/api/v1/stations/{stationId}", func(r chi.Router) {
		r.Post("/cookies", handler.UploadCookiesHandler)
		r.Get("/cookies/status", handler.GetCookiesStatusHandler)

		r.Group(func(r chi.Router) {
			r.Use(requireDJ)
			r.Post("/upload", s.Upload)
			r.Post("/youtube", s.YouTube)
			r.Get("/jobs", s.ListJobs)
			r.Get("/jobs/{jobId}", s.GetJob)
		})
	})
	r.With(handler.RequireAdmin).Post("/api/v1/admin/cookies", handler.SaveCookiesAdmin)
	return r
}

func requireDJ(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader || !auth.ValidateDJToken(token, chi.URLParam(r, "stationId")) {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Healthz responde 200 com a fila fora do ar: tirar a Media API do balanceador
// só trocaria um 503 explicado por um 404 do serviço que responde pelo resto.
func (s *Service) Healthz(w http.ResponseWriter, r *http.Request) {
	version := os.Getenv("APP_VERSION")
	if version == "" {
		version = "dev"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"service":        "media",
		"instance":       s.cfg.InstanceID,
		"version":        version,
		"queueConnected": s.cfg.QueueConnected == nil || s.cfg.QueueConnected(),
	})
}

// stationExists responde o erro e devolve false se a estação não puder receber
// o pedido.
func (s *Service) stationExists(w http.ResponseWriter, r *http.Request, stationID string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, err := s.cfg.Repo.GetStation(ctx, stationID)
	if errors.Is(err, storage.ErrStationNotFound) {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return false
	}
	if err != nil {
		log.Printf("[media] consultar estação %s: %v", stationID, err)
		http.Error(w, "banco de dados temporariamente indisponível", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// Upload guarda o original no ObjectStore e enfileira a conversão.
func (s *Service) Upload(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	if !s.stationExists(w, r, stationID) {
		return
	}

	// Folga para os cabeçalhos e os outros campos do multipart.
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+1<<20)
	if err := r.ParseMultipartForm(uploadMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			s.tooLarge(w)
			return
		}
		http.Error(w, "arquivo não enviado", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "arquivo não enviado", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch {
	case !media.IsSupported(ext):
		http.Error(w, fmt.Sprintf("formato não suportado: %s", ext), http.StatusBadRequest)
		return
	case header.Size == 0:
		http.Error(w, "o arquivo está vazio", http.StatusBadRequest)
		return
	case header.Size > s.cfg.MaxUpload:
		s.tooLarge(w)
		return
	}

	jobID := uuid.New().String()
	job := queue.Job{
		ID:         jobID,
		Kind:       queue.KindUpload,
		StationID:  stationID,
		Title:      uploadTitle(header),
		Category:   strings.TrimSpace(r.FormValue("category")),
		Theme:      strings.TrimSpace(r.FormValue("theme")),
		CoverURL:   strings.TrimSpace(r.FormValue("coverUrl")),
		SourceKey:  OriginalsPrefix + jobID + ext,
		EnqueuedAt: time.Now().UnixMilli(),
	}
	if err := s.cfg.Objects.Put(r.Context(), job.SourceKey, file, header.Size, "application/octet-stream"); err != nil {
		log.Printf("[media] guardar original do job %s: %v", job.ID, err)
		metrics.MediaJobsEnqueued.WithLabelValues(string(job.Kind), "error").Inc()
		http.Error(w, "armazenamento temporariamente indisponível; tente de novo", http.StatusServiceUnavailable)
		return
	}
	s.enqueue(w, r, job)
}

func (s *Service) tooLarge(w http.ResponseWriter) {
	http.Error(w, fmt.Sprintf("arquivo grande demais (máximo %d MB)", s.cfg.MaxUpload>>20), http.StatusRequestEntityTooLarge)
}

func uploadTitle(header *multipart.FileHeader) string {
	name := filepath.Base(header.Filename)
	if title := strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name))); title != "" {
		return title
	}
	return "Música enviada"
}

type youTubeRequest struct {
	URL      string `json:"url"`
	Category string `json:"category,omitempty"`
	Theme    string `json:"theme,omitempty"`
}

// YouTube enfileira o download de um vídeo.
func (s *Service) YouTube(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	if !s.stationExists(w, r, stationID) {
		return
	}

	var req youTubeRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "URL do YouTube inválida ou ausente", http.StatusBadRequest)
		return
	}
	if _, err := media.ValidateYouTubeURL(req.URL); err != nil {
		http.Error(w, "informe um link do YouTube (youtube.com ou youtu.be)", http.StatusBadRequest)
		return
	}
	videoID := media.YouTubeID(req.URL)
	if videoID == "" {
		http.Error(w, "informe o link de um vídeo do YouTube (playlists e canais não são aceitos)", http.StatusBadRequest)
		return
	}

	s.enqueue(w, r, queue.Job{
		ID:         uuid.New().String(),
		Kind:       queue.KindYouTube,
		StationID:  stationID,
		Category:   strings.TrimSpace(req.Category),
		Theme:      strings.TrimSpace(req.Theme),
		CoverURL:   fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", videoID),
		VideoURL:   media.WatchURL(videoID),
		VideoID:    videoID,
		EnqueuedAt: time.Now().UnixMilli(),
	})
}

// enqueue publica o job e responde 202. O registro é salvo antes: se fosse
// depois, poderia sobrescrever o andamento já gravado pelo worker.
func (s *Service) enqueue(w http.ResponseWriter, r *http.Request, job queue.Job) {
	rec := jobs.NewRecord(job, jobs.Queued)
	if err := s.cfg.Jobs.Save(r.Context(), rec); err != nil {
		log.Printf("[media] salvar o andamento do job %s: %v", job.ID, err)
	}

	ctx, span := tracing.StartProducer(r.Context(), "queue.publish "+queue.TranscodeQueue,
		attribute.String("messaging.system", "rabbitmq"),
		attribute.String("messaging.destination.name", queue.TranscodeQueue),
		attribute.String("radiosync.job.id", job.ID),
		attribute.String("radiosync.job.kind", string(job.Kind)),
		attribute.String("radiosync.station.id", job.StationID))
	defer span.End()
	job.Trace = tracing.Inject(ctx) // o worker continua este trace

	if err := s.cfg.Queue.Publish(ctx, job); err != nil {
		tracing.Fail(span, err)
		log.Printf("[media] publicar job %s: %v", job.ID, err)
		metrics.MediaJobsEnqueued.WithLabelValues(string(job.Kind), "error").Inc()
		// O original fica: sem confirmação, o broker pode ter recebido o job
		// mesmo assim, e o worker precisaria do arquivo.
		_ = s.cfg.Jobs.Delete(context.WithoutCancel(r.Context()), rec)
		w.Header().Set("Retry-After", "10")
		http.Error(w, "fila de processamento indisponível; tente de novo em instantes", http.StatusServiceUnavailable)
		return
	}

	metrics.MediaJobsEnqueued.WithLabelValues(string(job.Kind), "ok").Inc()
	w.Header().Set("Location", fmt.Sprintf("/api/v1/stations/%s/jobs/%s", job.StationID, job.ID))
	writeJSON(w, http.StatusAccepted, rec)
}

// ListJobs devolve os jobs recentes da estação.
func (s *Service) ListJobs(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Jobs.ListByStation(r.Context(), chi.URLParam(r, "stationId"))
	if err != nil {
		http.Error(w, "andamento temporariamente indisponível", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Service) GetJob(w http.ResponseWriter, r *http.Request) {
	rec, ok, err := s.cfg.Jobs.Get(r.Context(), chi.URLParam(r, "jobId"))
	if err != nil {
		http.Error(w, "andamento temporariamente indisponível", http.StatusServiceUnavailable)
		return
	}
	if !ok || rec.StationID != chi.URLParam(r, "stationId") {
		http.Error(w, "job não encontrado", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
