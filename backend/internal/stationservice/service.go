package stationservice

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/handler"
	"radio-backend/internal/metrics"
	"radio-backend/internal/model"
	"radio-backend/internal/security"
	"radio-backend/internal/storage"
	"radio-backend/internal/syncservice"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

var startedAt = time.Now()

func generateCode(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(codeChars)))
	for i := range b {
		num, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = codeChars[0]
		} else {
			b[i] = codeChars[num.Int64()]
		}
	}
	return string(b)
}

// Config reúne as dependências do Station Service (ADR-0003, ADR-0006).
type Config struct {
	InstanceID        string
	Repo              storage.Repository
	ObjectStore       storage.ObjectStore
	SyncStorage       syncservice.SyncStorage // Opcional: Redis para estado quente e lock de DJ
	AudioDeliveryMode string                  // "proxy" (padrão) ou "presigned"
	AllowedOrigins    []string
}

// Service implementa o Station Service extraído do monolito (Strangler Fig).
// Responsável por estações, playlist, biblioteca global, claim-dj, emissão do token de DJ
// e entrega de áudio a partir do MinIO sem depender de disco local.
type Service struct {
	instanceID        string
	repo              storage.Repository
	objectStore       storage.ObjectStore
	syncStorage       syncservice.SyncStorage
	audioDeliveryMode string
	allowedOrigins    []string
}

func NewService(cfg Config) *Service {
	if cfg.InstanceID == "" {
		cfg.InstanceID = "station-1"
	}
	if cfg.AudioDeliveryMode == "" {
		cfg.AudioDeliveryMode = os.Getenv("AUDIO_DELIVERY_MODE")
		if cfg.AudioDeliveryMode == "" {
			cfg.AudioDeliveryMode = "proxy"
		}
	}
	return &Service{
		instanceID:        cfg.InstanceID,
		repo:              cfg.Repo,
		objectStore:       cfg.ObjectStore,
		syncStorage:       cfg.SyncStorage,
		audioDeliveryMode: cfg.AudioDeliveryMode,
		allowedOrigins:    cfg.AllowedOrigins,
	}
}

// Router constrói o roteador HTTP do Station Service com todas as rotas migradas no gateway.
func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(security.CORS(s.allowedOrigins))

	r.Get("/healthz", s.Healthz)
	r.Handle("/metrics", metrics.Handler())

	// Áudio servido diretamente do MinIO (bucket "musicas"), sem disco local (ADR-0006)
	audioHandler := handler.ServeAudioFromObjectStore(s.objectStore, s.audioDeliveryMode)
	r.Get("/musicas/{filename}", audioHandler)
	r.Head("/musicas/{filename}", audioHandler)
	r.Get("/musicas/*", audioHandler)

	// Rotas /api/v1/stations* e /api/v1/library migradas via Strangler Fig (ADR-0003)
	r.Route("/api/v1/stations", func(r chi.Router) {
		r.Post("/", s.CreateStation)
		r.Get("/{stationId}", s.GetStation)
		r.Post("/{stationId}/claim-dj", s.ClaimDJRole)
		r.Get("/{stationId}/repository", s.GetRepository)
		r.Get("/{stationId}/musicas", s.ListMusicFiles)
		r.Get("/{stationId}/library", s.GetGlobalLibrary)
		r.Put("/{stationId}/tracks/{trackId}", s.UpdateTrackMetadata)
		r.Get("/{stationId}/playlist", s.GetPlaylist)
		r.Put("/{stationId}/playlist", s.UpdatePlaylist)
	})

	r.Get("/api/v1/library", s.GetGlobalLibrary)
	return r
}

func (s *Service) Healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := "ok"
	code := http.StatusOK
	if s.repo == nil {
		status = "repositório não configurado"
		code = http.StatusServiceUnavailable
	} else if err := s.repo.Ping(ctx); err != nil {
		status = "postgres indisponível"
		code = http.StatusServiceUnavailable
	}

	version := os.Getenv("APP_VERSION")
	if version == "" {
		version = "dev"
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        status,
		"service":       "station",
		"instance":      s.instanceID,
		"version":       version,
		"uptimeSeconds": int(time.Since(startedAt).Seconds()),
	})
}

// CreateStation cria uma nova estação no PostgreSQL e emite o JWT de DJ.
// Caso o PostgreSQL esteja fora do ar (RES-05), falha rápido com HTTP 503 e mensagem clara.
func (s *Service) CreateStation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.repo.Ping(ctx); err != nil {
		log.Printf("[station] falha ao criar estação (Postgres indisponível): %v", err)
		writeJSONError(w, "serviço de banco de dados (PostgreSQL) temporariamente indisponível; não foi possível criar a estação", http.StatusServiceUnavailable)
		return
	}

	id := generateCode(6)
	for attempts := 0; attempts < 10; attempts++ {
		_, err := s.repo.GetStation(ctx, id)
		if errors.Is(err, storage.ErrStationNotFound) {
			break
		}
		if err != nil {
			writeJSONError(w, "erro ao consultar disponibilidade da estação no banco de dados", http.StatusServiceUnavailable)
			return
		}
		id = generateCode(6)
	}

	token, err := auth.GenerateDJToken(id)
	if err != nil {
		http.Error(w, "erro ao gerar token", http.StatusInternalServerError)
		return
	}

	station := &model.Station{
		ID:          id,
		DJ:          token,
		State:       &model.PlaybackState{},
		Repository:  make([]model.Track, 0),
		Playlist:    make([]model.Track, 0),
		Suggestions: make([]model.SongSuggestion, 0),
		Mixer:       make([]model.MixerChannel, 0),
	}

	if err := s.repo.CreateStation(ctx, station); err != nil {
		log.Printf("[station] erro ao persistir nova estação %s: %v", id, err)
		writeJSONError(w, "não foi possível persistir a estação no banco de dados", http.StatusServiceUnavailable)
		return
	}

	if s.syncStorage != nil {
		_ = s.syncStorage.SavePlaybackState(ctx, id, station.State)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"stationId": id,
		"djToken":   token,
	})
}

// GetStation retorna os dados completos da estação combinando metadados do PostgreSQL
// com estado quente e presença de DJ do Redis (quando disponível), mantendo paridade
// exata com o contrato do monolito (INT-04, INT-05).
func (s *Service) GetStation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "stationId")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	station, err := s.repo.GetStation(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "banco de dados temporariamente indisponível", http.StatusServiceUnavailable)
		return
	}

	state := station.State
	if state == nil {
		state = &model.PlaybackState{}
	}
	hasDJ := false

	if s.syncStorage != nil {
		if hotState, err := s.syncStorage.GetPlaybackState(ctx, id); err == nil && hotState != nil {
			state = hotState
		}
		if online, err := s.syncStorage.IsDJOnline(ctx, id); err == nil {
			hasDJ = online
		}
	}

	authHeader := r.Header.Get("Authorization")
	reqToken := strings.TrimPrefix(authHeader, "Bearer ")
	if reqToken == authHeader {
		reqToken = ""
	}

	role := "listener"
	if reqToken != "" && auth.ValidateDJToken(reqToken, id) {
		role = "dj"
	}

	playlist := station.Playlist
	if playlist == nil {
		playlist = make([]model.Track, 0)
	}
	repoTracks := station.Repository
	if repoTracks == nil {
		repoTracks = make([]model.Track, 0)
	}
	suggestions := station.Suggestions
	if suggestions == nil {
		suggestions = make([]model.SongSuggestion, 0)
	}
	mixer := station.Mixer
	if mixer == nil {
		mixer = make([]model.MixerChannel, 0)
	}

	resp := map[string]interface{}{
		"id":          station.ID,
		"playlist":    playlist,
		"state":       state,
		"repository":  repoTracks,
		"suggestions": suggestions,
		"mixer":       mixer,
		"hasDJOnline": hasDJ,
		"role":        role,
	}

	w.Header().Set("Content-Type", "application/json")
	if role == "dj" {
		if renewed, err := auth.GenerateDJToken(id); err == nil {
			resp["djToken"] = renewed
			_ = s.repo.UpdateStationDJ(ctx, id, renewed)
		}
		w.Header().Set("Cache-Control", "no-store")
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// ClaimDJRole permite que um ouvinte assuma a bancada de DJ caso não haja DJ conectado globalmente.
func (s *Service) ClaimDJRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "stationId")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := s.repo.GetStation(ctx, id); err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "banco de dados temporariamente indisponível", http.StatusServiceUnavailable)
		return
	}

	if s.syncStorage != nil {
		if hasDJ, err := s.syncStorage.IsDJOnline(ctx, id); err == nil && hasDJ {
			http.Error(w, "já existe um DJ conectado nesta estação no momento", http.StatusConflict)
			return
		}
	}

	token, err := auth.GenerateDJToken(id)
	if err != nil {
		http.Error(w, "erro ao gerar token", http.StatusInternalServerError)
		return
	}

	if err := s.repo.UpdateStationDJ(ctx, id, token); err != nil {
		writeJSONError(w, "erro ao atualizar DJ da estação", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"stationId": id,
		"djToken":   token,
	})
}

// GetRepository lista as faixas do repositório da estação (exclusivo para o DJ autenticado).
func (s *Service) GetRepository(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	if !validateBearerDJ(r, stationID) {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	station, err := s.repo.GetStation(ctx, stationID)
	if err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "banco de dados indisponível", http.StatusServiceUnavailable)
		return
	}

	tracks := station.Repository
	if tracks == nil {
		tracks = make([]model.Track, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tracks)
}

// ListMusicFiles lista os arquivos .opus disponíveis no bucket "musicas" do MinIO.
func (s *Service) ListMusicFiles(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := s.repo.GetStation(ctx, stationID); err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "banco de dados indisponível", http.StatusServiceUnavailable)
		return
	}

	files := make([]string, 0)
	if s.objectStore != nil {
		keys, err := s.objectStore.ListKeys(ctx, "")
		if err == nil {
			for _, k := range keys {
				if strings.HasSuffix(strings.ToLower(k), ".opus") {
					files = append(files, k)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(files)
}

// GetGlobalLibrary retorna todas as músicas catalogadas no PostgreSQL (/api/v1/library).
func (s *Service) GetGlobalLibrary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tracks, err := s.repo.ListLibraryTracks(ctx)
	if err != nil {
		writeJSONError(w, "erro ao consultar biblioteca global", http.StatusServiceUnavailable)
		return
	}
	if tracks == nil {
		tracks = make([]model.Track, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tracks)
}

// UpdateTrackMetadata atualiza título, categoria, tema e capa de uma faixa da estação no PostgreSQL.
func (s *Service) UpdateTrackMetadata(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	trackID := chi.URLParam(r, "trackId")

	if !validateBearerDJ(r, stationID) {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}

	var req handler.UpdateTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "payload inválido", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	station, err := s.repo.GetStation(ctx, stationID)
	if err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "banco de dados indisponível", http.StatusServiceUnavailable)
		return
	}

	var updated model.Track
	found := false
	for _, t := range station.Repository {
		if t.ID == trackID {
			updated = t
			if strings.TrimSpace(req.Title) != "" {
				updated.Title = strings.TrimSpace(req.Title)
			}
			updated.Category = strings.TrimSpace(req.Category)
			updated.Theme = strings.TrimSpace(req.Theme)
			if strings.TrimSpace(req.CoverURL) != "" {
				updated.CoverURL = strings.TrimSpace(req.CoverURL)
			}
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "música não encontrada no repositório", http.StatusNotFound)
		return
	}

	if err := s.repo.UpdateStationTrack(ctx, stationID, updated); err != nil {
		writeJSONError(w, "erro ao atualizar metadados da faixa", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

// GetPlaylist retorna a playlist ordenada da estação.
func (s *Service) GetPlaylist(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := s.repo.GetStation(ctx, stationID); err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "banco de dados indisponível", http.StatusServiceUnavailable)
		return
	}

	pl, err := s.repo.GetPlaylist(ctx, stationID)
	if err != nil {
		writeJSONError(w, "erro ao buscar playlist", http.StatusServiceUnavailable)
		return
	}
	if pl == nil {
		pl = make([]model.Track, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pl)
}

// UpdatePlaylist substitui a playlist da estação no PostgreSQL e propaga PLAYLIST_UPDATED no Redis.
func (s *Service) UpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	if !validateBearerDJ(r, stationID) {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}

	var req struct {
		Playlist []model.Track `json:"playlist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "payload inválido", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.repo.ReplacePlaylist(ctx, stationID, req.Playlist); err != nil {
		if errors.Is(err, storage.ErrStationNotFound) {
			http.Error(w, "estação não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "erro ao salvar playlist", http.StatusServiceUnavailable)
		return
	}

	if s.syncStorage != nil {
		plPayload, _ := json.Marshal(req.Playlist)
		_ = s.syncStorage.PublishEvent(ctx, stationID, &syncservice.RedisEvent{
			SenderID:  s.instanceID,
			StationID: stationID,
			Type:      "PLAYLIST_UPDATED",
			Payload:   plPayload,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req.Playlist)
}

func validateBearerDJ(r *http.Request, stationID string) bool {
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader {
		return false
	}
	return auth.ValidateDJToken(token, stationID)
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
	})
}
