package mediaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/jobs"
	"radio-backend/internal/mediaservice"
	"radio-backend/internal/model"
	"radio-backend/internal/queue"
	"radio-backend/internal/stationservice"
	"radio-backend/internal/syncservice"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// TestINT02_UploadFilaWorkerTrackReady valida o INT-02 de ponta a ponta:
// upload na Media API -> fila -> worker -> TRACK_READY no WebSocket do Sync
// Service -> faixa no repositório lido pelo Station Service. A conversão leva
// 1 s e o 202 precisa sair em menos de 500 ms.
func TestINT02_UploadFilaWorkerTrackReady(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-int02-com-32-bytes!!")
	const station = "INT002"
	a := novoAmbiente(t, station)
	token, err := auth.GenerateDJToken(station)
	if err != nil {
		t.Fatal(err)
	}

	broker := syncservice.NewMemoryBroker()
	syncSvc := syncservice.NewSyncService(syncservice.NewMemorySyncStorageWithBroker("sync-1", broker), "sync-1", nil)
	defer syncSvc.Close()
	r := chi.NewRouter()
	r.Get("/ws/stations/{stationId}", syncSvc.HandleWebSocket)
	syncSrv := httptest.NewServer(r)
	defer syncSrv.Close()

	mediaSrv := httptest.NewServer(mediaservice.NewService(mediaservice.Config{
		Repo: a.repo, Objects: a.objects, Queue: a.queue, Jobs: a.jobs,
	}).Router())
	defer mediaSrv.Close()

	stationSrv := httptest.NewServer(stationservice.NewService(stationservice.Config{
		Repo: a.repo, ObjectStore: a.objects,
	}).Router())
	defer stationSrv.Close()

	cfg := a.config("worker-1")
	cfg.Events = syncservice.NewMemorySyncStorageWithBroker("worker-1", broker)
	cfg.LockTTL = 30 * time.Second
	base := cfg.Transcode
	cfg.Transcode = func(ctx context.Context, in, out string) (float64, error) {
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		return base(ctx, in, out)
	}
	inicia(t, cfg)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(syncSrv.URL, "http")+"/ws/stations/"+station, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 1. Upload: 202 rápido, com jobId.
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", "Minha Música.mp3")
	_, _ = part.Write([]byte("conteúdo mp3"))
	_ = form.WriteField("category", "Rock")
	_ = form.Close()
	req, _ := http.NewRequest(http.MethodPost, mediaSrv.URL+"/api/v1/stations/"+station+"/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	inicio := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	latencia := time.Since(inicio)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d, esperado 202", resp.StatusCode)
	}
	if latencia >= 500*time.Millisecond {
		t.Fatalf("202 em %v, esperado < 500 ms", latencia)
	}
	var aceito jobs.Record
	if err := json.NewDecoder(resp.Body).Decode(&aceito); err != nil || aceito.ID == "" || aceito.Status != jobs.Queued {
		t.Fatalf("resposta: %+v err=%v", aceito, err)
	}
	if repo := a.repositorio(t, station); len(repo) != 0 {
		t.Fatal("a faixa não deveria aparecer antes da conversão")
	}

	// 2. TRACK_READY chega pelo WebSocket.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ev jobs.Event
	for ev.Type != jobs.EventTrackReady {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("TRACK_READY não chegou: %v", err)
		}
		ev = jobs.Event{}
		_ = json.Unmarshal(msg, &ev)
	}
	t.Logf("202 em %v; TRACK_READY em %v", latencia.Round(time.Microsecond), time.Since(inicio).Round(time.Millisecond))
	if ev.JobID != aceito.ID || ev.Track == nil || ev.Track.Title != "Minha Música" || ev.Track.Category != "Rock" {
		t.Fatalf("evento: %+v track=%+v", ev, ev.Track)
	}

	// 3. A faixa está no repositório e o áudio toca pelo Station Service.
	reqRepo, _ := http.NewRequest(http.MethodGet, stationSrv.URL+"/api/v1/stations/"+station+"/repository", nil)
	reqRepo.Header.Set("Authorization", "Bearer "+token)
	respRepo, err := http.DefaultClient.Do(reqRepo)
	if err != nil {
		t.Fatal(err)
	}
	defer respRepo.Body.Close()
	var tracks []model.Track
	_ = json.NewDecoder(respRepo.Body).Decode(&tracks)
	if len(tracks) != 1 || tracks[0].ID != aceito.ID {
		t.Fatalf("repositório: %+v", tracks)
	}
	respAudio, err := http.Get(stationSrv.URL + tracks[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	respAudio.Body.Close()
	if respAudio.StatusCode != http.StatusOK {
		t.Fatalf("áudio: status %d", respAudio.StatusCode)
	}

	// 4. O andamento consultado pela API também está pronto.
	reqJob, _ := http.NewRequest(http.MethodGet, mediaSrv.URL+resp.Header.Get("Location"), nil)
	reqJob.Header.Set("Authorization", "Bearer "+token)
	respJob, err := http.DefaultClient.Do(reqJob)
	if err != nil {
		t.Fatal(err)
	}
	defer respJob.Body.Close()
	var rec jobs.Record
	if err := json.NewDecoder(respJob.Body).Decode(&rec); err != nil || rec.Status != jobs.Ready || rec.Track == nil {
		t.Fatalf("andamento: %+v err=%v", rec, err)
	}
}

// TRACK_FAILED percorre o mesmo caminho até o WebSocket.
func TestTrackFailedChegaPeloWebSocket(t *testing.T) {
	const station = "FAIL01"
	a := novoAmbiente(t, station)
	broker := syncservice.NewMemoryBroker()
	syncSvc := syncservice.NewSyncService(syncservice.NewMemorySyncStorageWithBroker("sync-1", broker), "sync-1", nil)
	defer syncSvc.Close()
	srv := httptest.NewServer(http.HandlerFunc(syncSvc.HandleWebSocket))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/stations/"+station, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil { // inscrito no canal
		t.Fatal(err)
	}

	cfg := a.config("worker-1")
	cfg.Events = syncservice.NewMemorySyncStorageWithBroker("worker-1", broker)
	cfg.Retry = queue.RetryPolicy{Delays: []time.Duration{time.Millisecond}}
	cfg.Transcode = func(context.Context, string, string) (float64, error) { return 0, errors.New("x") }
	inicia(t, cfg)
	job := a.upload(t, "job1", station)

	var ev jobs.Event
	for ev.Type != jobs.EventTrackFailed {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("TRACK_FAILED não chegou: %v", err)
		}
		ev = jobs.Event{}
		_ = json.Unmarshal(msg, &ev)
	}
	if ev.JobID != job.ID || ev.Error == "" || ev.Title != job.Title {
		t.Fatalf("evento: %+v", ev)
	}
}
