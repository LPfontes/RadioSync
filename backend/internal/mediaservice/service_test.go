package mediaservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"radio-backend/internal/auth"
	"radio-backend/internal/jobs"
	"radio-backend/internal/model"
	"radio-backend/internal/queue"
	"radio-backend/internal/storage"
)

const station = "MEDIA1"

type ambiente struct {
	repo    *storage.MemoryRepository
	objects *storage.MemoryObjectStore
	queue   *queue.Memory
	jobs    *jobs.MemoryStore
	handler http.Handler
	token   string
}

func novo(t *testing.T, maxUpload int64) *ambiente {
	t.Helper()
	t.Setenv("JWT_SECRET", "segredo-de-teste-media-com-32-bytes!!")
	a := &ambiente{
		repo:    storage.NewMemoryRepository(),
		objects: storage.NewMemoryObjectStore(storage.DefaultMusicBucket),
		queue:   queue.NewMemory(),
		jobs:    jobs.NewMemoryStore(),
	}
	if err := a.repo.CreateStation(context.Background(), &model.Station{ID: station}); err != nil {
		t.Fatal(err)
	}
	a.handler = NewService(Config{Repo: a.repo, Objects: a.objects, Queue: a.queue, Jobs: a.jobs, MaxUpload: maxUpload}).Router()
	a.token, _ = auth.GenerateDJToken(station)
	return a
}

func (a *ambiente) do(method, path, contentType string, body io.Reader, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, req)
	return w
}

func (a *ambiente) upload(filename string, content []byte, token string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", filename)
	_, _ = part.Write(content)
	_ = form.Close()
	return a.do(http.MethodPost, "/api/v1/stations/"+station+"/upload", form.FormDataContentType(), &body, token)
}

func (a *ambiente) youtube(url string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"url": url, "category": " Rock "})
	return a.do(http.MethodPost, "/api/v1/stations/"+station+"/youtube", "application/json", bytes.NewReader(body), a.token)
}

func (a *ambiente) originais(t *testing.T) []string {
	t.Helper()
	keys, err := a.objects.ListKeys(context.Background(), OriginalsPrefix)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestUpload_Aceito(t *testing.T) {
	a := novo(t, 0)
	w := a.upload("pasta/Minha Faixa.MP3", []byte("abc"), a.token)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var rec jobs.Record
	_ = json.Unmarshal(w.Body.Bytes(), &rec)
	if rec.Status != jobs.Queued || rec.Title != "Minha Faixa" || rec.StationID != station {
		t.Fatalf("registro: %+v", rec)
	}
	if w.Header().Get("Location") != "/api/v1/stations/"+station+"/jobs/"+rec.ID {
		t.Errorf("Location: %q", w.Header().Get("Location"))
	}
	if keys := a.originais(t); len(keys) != 1 || keys[0] != OriginalsPrefix+rec.ID+".mp3" {
		t.Fatalf("originais: %v", keys)
	}
	if a.queue.Ready() != 1 {
		t.Fatal("o job não foi publicado")
	}
}

func TestYouTube_Aceito(t *testing.T) {
	a := novo(t, 0)
	if w := a.youtube("https://youtu.be/dQw4w9WgXcQ?list=PL1"); w.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan queue.Job, 1)
	go func() {
		_ = a.queue.Consume(ctx, 1, func(_ context.Context, d *queue.Delivery) { got <- d.Job; _ = d.Ack() })
	}()
	job := <-got
	if job.VideoID != "dQw4w9WgXcQ" || job.VideoURL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" || job.Category != "Rock" {
		t.Fatalf("job: %+v", job)
	}
	if err := job.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRecusados(t *testing.T) {
	a := novo(t, 1<<20)
	outra, _ := auth.GenerateDJToken("OUTRA1")
	semEstacao, _ := auth.GenerateDJToken("NAOEXI")

	casos := map[string]struct {
		w    *httptest.ResponseRecorder
		want int
	}{
		"upload sem token":           {a.upload("a.mp3", []byte("x"), ""), 401},
		"upload com outra estação":   {a.upload("a.mp3", []byte("x"), outra), 401},
		"formato não suportado":      {a.upload("a.exe", []byte("x"), a.token), 400},
		"arquivo vazio":              {a.upload("a.mp3", nil, a.token), 400},
		"grande demais":              {a.upload("a.mp3", make([]byte, 1<<20+1), a.token), 413},
		"muito grande demais":        {a.upload("a.mp3", make([]byte, 3<<20), a.token), 413},
		"corpo não é multipart":      {a.do("POST", "/api/v1/stations/"+station+"/upload", "text/plain", strings.NewReader("x"), a.token), 400},
		"estação inexistente":        {a.do("POST", "/api/v1/stations/NAOEXI/youtube", "", strings.NewReader("{}"), semEstacao), 404},
		"youtube sem url":            {a.youtube(""), 400},
		"youtube outro site":         {a.youtube("https://vimeo.com/1"), 400},
		"youtube opção do yt-dlp":    {a.youtube("--version"), 400},
		"youtube playlist sem vídeo": {a.youtube("https://www.youtube.com/playlist?list=PL1"), 400},
		"jobs sem token":             {a.do("GET", "/api/v1/stations/"+station+"/jobs", "", nil, ""), 401},
		"job inexistente":            {a.do("GET", "/api/v1/stations/"+station+"/jobs/x", "", nil, a.token), 404},
		"cookies sem token":          {a.do("POST", "/api/v1/stations/"+station+"/cookies", "", strings.NewReader(`{"content":"x"}`), ""), 401},
		"cookies admin sem senha":    {a.do("POST", "/api/v1/admin/cookies", "", strings.NewReader(`{"content":"x"}`), ""), 503},
	}
	for nome, c := range casos {
		if c.w.Code != c.want {
			t.Errorf("%s: status %d, esperado %d (%s)", nome, c.w.Code, c.want, strings.TrimSpace(c.w.Body.String()))
		}
	}
	if a.queue.Ready() != 0 || len(a.originais(t)) != 0 {
		t.Fatalf("pedido recusado deixou rastro: fila=%d originais=%v", a.queue.Ready(), a.originais(t))
	}
}

func TestDependenciasForaDoAr(t *testing.T) {
	t.Run("fila", func(t *testing.T) {
		a := novo(t, 0)
		a.queue.SetAvailable(false)
		w := a.upload("a.mp3", []byte("x"), a.token)
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
			t.Fatalf("status %d", w.Code)
		}
		if list, _ := a.jobs.ListByStation(context.Background(), station); len(list) != 0 {
			t.Fatalf("o job recusado não pode ficar como 'processando': %+v", list)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		a := novo(t, 0)
		a.repo.SetAvailable(false)
		if w := a.upload("a.mp3", []byte("x"), a.token); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %d", w.Code)
		}
	})
	t.Run("redis não impede o upload", func(t *testing.T) {
		a := novo(t, 0)
		a.jobs.SetAvailable(false)
		if w := a.upload("a.mp3", []byte("x"), a.token); w.Code != http.StatusAccepted || a.queue.Ready() != 1 {
			t.Fatalf("status %d, fila %d", w.Code, a.queue.Ready())
		}
	})
	t.Run("minio", func(t *testing.T) {
		a := novo(t, 0)
		a.handler = NewService(Config{Repo: a.repo, Objects: semPut{a.objects}, Queue: a.queue, Jobs: a.jobs}).Router()
		if w := a.upload("a.mp3", []byte("x"), a.token); w.Code != http.StatusServiceUnavailable || a.queue.Ready() != 0 {
			t.Fatalf("status %d, fila %d", w.Code, a.queue.Ready())
		}
	})
}

type semPut struct{ storage.ObjectStore }

func (semPut) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("minio fora do ar")
}

// Um DJ não enxerga os jobs de outra estação.
func TestJobs_IsoladosPorEstacao(t *testing.T) {
	a := novo(t, 0)
	ctx := context.Background()
	_ = a.jobs.Save(ctx, jobs.Record{ID: "meu", StationID: station, Status: jobs.Failed, Error: "x"})
	_ = a.jobs.Save(ctx, jobs.Record{ID: "alheio", StationID: "OUTRA1"})

	w := a.do("GET", "/api/v1/stations/"+station+"/jobs", "", nil, a.token)
	var list []jobs.Record
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if w.Code != 200 || len(list) != 1 || list[0].ID != "meu" {
		t.Fatalf("status %d: %+v", w.Code, list)
	}
	if w := a.do("GET", "/api/v1/stations/"+station+"/jobs/alheio", "", nil, a.token); w.Code != http.StatusNotFound {
		t.Fatalf("job de outra estação: status %d", w.Code)
	}
}
