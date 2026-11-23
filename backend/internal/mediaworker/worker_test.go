package mediaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"radio-backend/internal/breaker"
	"radio-backend/internal/jobs"
	"radio-backend/internal/media"
	"radio-backend/internal/model"
	"radio-backend/internal/queue"
	"radio-backend/internal/storage"
	"radio-backend/internal/syncservice"
)

// ---------------------------------------------------------------- ambiente

type eventos struct {
	mu   sync.Mutex
	list []jobs.Event
}

func (e *eventos) PublishEvent(_ context.Context, stationID string, ev *syncservice.RedisEvent) error {
	var msg jobs.Event
	if err := json.Unmarshal(ev.Payload, &msg); err != nil {
		return err
	}
	e.mu.Lock()
	e.list = append(e.list, msg)
	e.mu.Unlock()
	return nil
}

func (e *eventos) tipos() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := make(map[string]int)
	for _, ev := range e.list {
		n[ev.Type]++
	}
	return n
}

type ambiente struct {
	queue   *queue.Memory
	jobs    *jobs.MemoryStore
	repo    *storage.MemoryRepository
	objects *storage.MemoryObjectStore
	events  *eventos

	conversoes, downloads atomic.Int32
}

var retryRapido = queue.RetryPolicy{Delays: []time.Duration{time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}}

func novoAmbiente(t *testing.T, stations ...string) *ambiente {
	t.Helper()
	a := &ambiente{
		queue:   queue.NewMemory(),
		jobs:    jobs.NewMemoryStore(),
		repo:    storage.NewMemoryRepository(),
		objects: storage.NewMemoryObjectStore(storage.DefaultMusicBucket),
		events:  &eventos{},
	}
	for _, id := range stations {
		if err := a.repo.CreateStation(context.Background(), &model.Station{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// config devolve um worker com FFmpeg e yt-dlp falsos que dão certo.
func (a *ambiente) config(id string) Config {
	return Config{
		ID: id, Queue: a.queue, Jobs: a.jobs, Repo: a.repo, Objects: a.objects, Events: a.events,
		Retry:   retryRapido,
		LockTTL: 100 * time.Millisecond,
		Transcode: func(_ context.Context, input, output string) (float64, error) {
			a.conversoes.Add(1)
			data, err := os.ReadFile(input)
			if err != nil {
				return 0, err
			}
			return 12.5, os.WriteFile(output, append([]byte("OggS:"), data...), 0600)
		},
		Download: func(_ context.Context, _, output string) (string, float64, error) {
			a.downloads.Add(1)
			return "Título do vídeo", 200, os.WriteFile(output, []byte("OggS"), 0600)
		},
	}
}

// inicia roda o worker e devolve a função que o mata sem esperar (kill -9).
func inicia(t *testing.T, cfg Config) context.CancelFunc {
	t.Helper()
	ctx, kill := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = New(cfg).Run(ctx)
	}()
	t.Cleanup(func() { kill(); <-done })
	return kill
}

func (a *ambiente) upload(t *testing.T, id, station string) queue.Job {
	t.Helper()
	job := queue.Job{ID: id, Kind: queue.KindUpload, StationID: station, Title: "Faixa " + id,
		SourceKey: "originals/" + id + ".mp3", EnqueuedAt: time.Now().UnixMilli()}
	if err := a.objects.Put(context.Background(), job.SourceKey, bytes.NewReader([]byte("mp3")), 3, ""); err != nil {
		t.Fatal(err)
	}
	a.publica(t, job)
	return job
}

func (a *ambiente) youtube(t *testing.T, id, station, videoID string) queue.Job {
	t.Helper()
	job := queue.Job{ID: id, Kind: queue.KindYouTube, StationID: station,
		VideoID: videoID, VideoURL: media.WatchURL(videoID), EnqueuedAt: time.Now().UnixMilli()}
	a.publica(t, job)
	return job
}

func (a *ambiente) publica(t *testing.T, job queue.Job) {
	t.Helper()
	if err := a.queue.Publish(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func espera(t *testing.T, oQue string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("não aconteceu em 5 s: %s", oQue)
}

func (a *ambiente) esperaStatus(t *testing.T, jobID string, want jobs.Status) jobs.Record {
	t.Helper()
	var rec jobs.Record
	espera(t, fmt.Sprintf("job %s ficar %s", jobID, want), func() bool {
		rec, _, _ = a.jobs.Get(context.Background(), jobID)
		return rec.Status == want
	})
	return rec
}

func (a *ambiente) repositorio(t *testing.T, station string) []model.Track {
	t.Helper()
	st, err := a.repo.GetStation(context.Background(), station)
	if err != nil {
		t.Fatal(err)
	}
	return st.Repository
}

// ---------------------------------------------------------------- caminho feliz

func TestUpload_ConverteESalva(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	inicia(t, a.config("w1"))
	job := a.upload(t, "job1", "EST001")

	rec := a.esperaStatus(t, "job1", jobs.Ready)
	repo := a.repositorio(t, "EST001")
	if len(repo) != 1 || repo[0].ID != "job1" || repo[0].Title != "Faixa job1" || repo[0].Duration != 12.5 {
		t.Fatalf("repositório: %+v", repo)
	}
	if rec.Track == nil || rec.Track.URL != "/musicas/job1.opus" {
		t.Fatalf("registro: %+v", rec)
	}
	if ok, _ := a.objects.Exists(context.Background(), "job1.opus"); !ok {
		t.Fatal("o .opus não foi para o ObjectStore")
	}
	espera(t, "apagar o original", func() bool {
		ok, _ := a.objects.Exists(context.Background(), job.SourceKey)
		return !ok
	})
	espera(t, "TRACK_READY", func() bool { return a.events.tipos()[jobs.EventTrackReady] == 1 })
}

// ---------------------------------------------------------------- idempotência

func TestIdempotencia_MesmoJobNaoConverteDuasVezes(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	inicia(t, a.config("w1"))
	job := a.upload(t, "job1", "EST001")
	a.esperaStatus(t, "job1", jobs.Ready)

	// Reentrega do mesmo job (ex.: o ack se perdeu).
	a.publica(t, job)
	espera(t, "segundo TRACK_READY", func() bool { return a.events.tipos()[jobs.EventTrackReady] == 2 })
	if n := a.conversoes.Load(); n != 1 {
		t.Fatalf("%d conversões, esperado 1", n)
	}
	if repo := a.repositorio(t, "EST001"); len(repo) != 1 {
		t.Fatalf("faixa duplicada: %+v", repo)
	}
}

func TestIdempotencia_JobEmAndamentoEmOutroWorkerEspera(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfg := a.config("w")
	cfg.Concurrency = 1
	entrou, libera := make(chan struct{}), make(chan struct{})
	var simultaneas, maximo atomic.Int32
	base := cfg.Transcode
	cfg.Transcode = func(ctx context.Context, in, out string) (float64, error) {
		if n := simultaneas.Add(1); n > maximo.Load() {
			maximo.Store(n)
		}
		defer simultaneas.Add(-1)
		close(entrou)
		<-libera
		return base(ctx, in, out)
	}
	job := a.upload(t, "job1", "EST001")
	inicia(t, cfg)
	<-entrou
	inicia(t, cfg)
	a.publica(t, job) // duplicata enquanto o primeiro worker ainda converte
	espera(t, "a duplicata ser adiada", func() bool { return len(a.queue.Delays()) > 2 })
	close(libera)

	espera(t, "os dois avisos", func() bool { return a.events.tipos()[jobs.EventTrackReady] == 2 })
	if a.conversoes.Load() != 1 || maximo.Load() != 1 {
		t.Fatalf("conversões=%d simultâneas=%d, esperado 1 e 1", a.conversoes.Load(), maximo.Load())
	}
	if rec, _, _ := a.jobs.Get(context.Background(), "job1"); rec.Attempt != 0 {
		t.Fatalf("esperar o lock não pode gastar tentativas: %+v", rec)
	}
}

func TestIdempotencia_MesmoVideoEmDuasEstacoes(t *testing.T) {
	a := novoAmbiente(t, "EST001", "EST002")
	a.youtube(t, "job1", "EST001", "dQw4w9WgXcQ")
	a.youtube(t, "job2", "EST002", "dQw4w9WgXcQ")
	inicia(t, a.config("w1"))
	inicia(t, a.config("w2"))

	a.esperaStatus(t, "job1", jobs.Ready)
	a.esperaStatus(t, "job2", jobs.Ready)
	if n := a.downloads.Load(); n != 1 {
		t.Fatalf("%d downloads do mesmo vídeo, esperado 1", n)
	}
	r1, r2 := a.repositorio(t, "EST001"), a.repositorio(t, "EST002")
	if len(r1) != 1 || len(r2) != 1 || r1[0].ID != r2[0].ID {
		t.Fatalf("as duas estações deveriam ter a mesma faixa: %+v e %+v", r1, r2)
	}
}

// O cache aponta para uma faixa cujo áudio sumiu: baixa de novo no mesmo ID.
func TestYouTube_CacheSemAudioConsertaAFaixa(t *testing.T) {
	ctx := context.Background()
	a := novoAmbiente(t, "EST001")
	_ = a.repo.UpsertTrack(ctx, model.Track{ID: "antiga", Title: "Renomeada pelo DJ", Filename: "antiga.opus", URL: "/musicas/antiga.opus"})
	_ = a.repo.SaveYouTubeCache(ctx, "dQw4w9WgXcQ", "antiga")
	inicia(t, a.config("w1"))
	a.youtube(t, "job1", "EST001", "dQw4w9WgXcQ")

	rec := a.esperaStatus(t, "job1", jobs.Ready)
	if rec.Track.ID != "antiga" || rec.Track.Title != "Renomeada pelo DJ" {
		t.Fatalf("faixa: %+v", rec.Track)
	}
	if ok, _ := a.objects.Exists(ctx, "antiga.opus"); !ok || a.downloads.Load() != 1 {
		t.Fatalf("áudio restaurado=%v downloads=%d", ok, a.downloads.Load())
	}
}

// ---------------------------------------------------------------- RES-02

// Worker morto no meio da conversão: a mensagem, sem ack, volta para a fila e
// outro worker a processa. O worker morto nunca publica a faixa.
func TestRES02_WorkerMortoNoMeioDaConversao(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfgA := a.config("w-a")
	entrou := make(chan struct{})
	cfgA.Transcode = func(ctx context.Context, _, _ string) (float64, error) {
		close(entrou)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	// Um kill -9 não solta o lock: ele só expira por tempo.
	cfgA.Jobs = semRelease{a.jobs}
	mata := inicia(t, cfgA)
	a.upload(t, "job1", "EST001")
	<-entrou

	inicio := time.Now()
	mata()
	espera(t, "a mensagem voltar para a fila", func() bool { return a.queue.Ready() == 1 })
	inicia(t, a.config("w-b"))
	a.esperaStatus(t, "job1", jobs.Ready)
	t.Logf("reprocessado %v depois da morte (lock de %v)", time.Since(inicio).Round(time.Millisecond), cfgA.LockTTL)

	if n := a.conversoes.Load(); n != 1 {
		t.Fatalf("%d conversões concluídas, esperado 1", n)
	}
	if repo := a.repositorio(t, "EST001"); len(repo) != 1 {
		t.Fatalf("repositório: %+v", repo)
	}
	if len(a.queue.Dead()) != 0 {
		t.Fatal("a morte do worker não pode mandar o job para a DLQ")
	}
}

type semRelease struct{ jobs.Store }

func (s semRelease) Lock(ctx context.Context, key string, ttl time.Duration) (jobs.Lock, bool, error) {
	l, ok, err := s.Store.Lock(ctx, key, ttl)
	if l != nil {
		l = lockPreso{l}
	}
	return l, ok, err
}

type lockPreso struct{ jobs.Lock }

func (lockPreso) Release(context.Context) error { return nil }

// Um worker travado (sem renovar o lock) perde o job e não pode publicar.
func TestLockPerdidoCancelaOTrabalho(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfg := a.config("w-a")
	base := cfg.Transcode
	var primeira atomic.Bool
	cfg.Jobs = lockQueSome{Store: a.jobs, primeira: &primeira}
	cancelada := make(chan error, 1)
	cfg.Transcode = func(ctx context.Context, in, out string) (float64, error) {
		if primeira.CompareAndSwap(false, true) {
			<-ctx.Done()
			cancelada <- context.Cause(ctx)
			return 0, ctx.Err()
		}
		return base(ctx, in, out)
	}
	inicia(t, cfg)
	a.upload(t, "job1", "EST001")
	select {
	case err := <-cancelada:
		if !errors.Is(err, errLockLost) {
			t.Fatalf("causa: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("o trabalho deveria ser cancelado ao perder o lock")
	}
	rec := a.esperaStatus(t, "job1", jobs.Ready)
	if rec.Attempt != 0 {
		t.Fatalf("perder o lock não gasta tentativa: %+v", rec)
	}
}

type lockQueSome struct {
	jobs.Store
	primeira *atomic.Bool
}

func (s lockQueSome) Lock(ctx context.Context, key string, ttl time.Duration) (jobs.Lock, bool, error) {
	l, ok, err := s.Store.Lock(ctx, key, ttl)
	if ok && !s.primeira.Load() {
		return perdido{l}, ok, err
	}
	return l, ok, err
}

type perdido struct{ jobs.Lock }

func (perdido) Refresh(context.Context, time.Duration) (bool, error) { return false, nil }

// ---------------------------------------------------------------- RES-03

// yt-dlp falhando sempre: 3 novas tentativas, DLQ, TRACK_FAILED e, depois de 5
// falhas seguidas, circuito aberto: o yt-dlp deixa de ser chamado.
func TestRES03_YtDlpFalhandoSempre(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	var agora atomic.Int64
	agora.Store(time.Now().UnixNano())
	var mu sync.Mutex
	var transicoes []string

	cfg := a.config("w1")
	cfg.Concurrency = 1
	base := cfg.Download
	var quebrado atomic.Bool
	quebrado.Store(true)
	cfg.Download = func(ctx context.Context, url, out string) (string, float64, error) {
		if quebrado.Load() {
			a.downloads.Add(1)
			return "", 0, errors.New("HTTP Error 429")
		}
		return base(ctx, url, out)
	}
	cfg.Breaker = NewBreaker(breaker.Config{
		MaxFailures: 5,
		Cooldown:    time.Minute,
		Now:         func() time.Time { return time.Unix(0, agora.Load()) },
		OnStateChange: func(from, to breaker.State) {
			mu.Lock()
			transicoes = append(transicoes, to.String())
			mu.Unlock()
		},
	})
	inicia(t, cfg)

	// Job 1: 1 tentativa + 3 novas, com o backoff da política, e DLQ.
	a.youtube(t, "job1", "EST001", "aaaaaaaaaaa")
	rec := a.esperaStatus(t, "job1", jobs.Failed)
	if n := a.downloads.Load(); n != 4 {
		t.Fatalf("%d chamadas ao yt-dlp, esperado 4", n)
	}
	if got := a.queue.Delays(); !reflect.DeepEqual(got, retryRapido.Delays) {
		t.Fatalf("atrasos %v, esperado %v", got, retryRapido.Delays)
	}
	dead := a.queue.Dead()
	if len(dead) != 1 || dead[0].Job.ID != "job1" || dead[0].Job.Attempt != 3 {
		t.Fatalf("DLQ: %+v", dead)
	}
	if rec.Error == "" || rec.Attempt != 3 {
		t.Fatalf("registro: %+v", rec)
	}
	if bytes.Contains([]byte(rec.Error), []byte("429")) {
		t.Fatalf("o erro técnico não deveria chegar ao DJ: %q", rec.Error)
	}
	if cfg.Breaker.State() != breaker.Closed {
		t.Fatalf("4 falhas não abrem o circuito, estado %s", cfg.Breaker.State())
	}

	// Job 2: a 5ª falha abre o circuito; as novas tentativas falham rápido.
	a.youtube(t, "job2", "EST001", "bbbbbbbbbbb")
	a.esperaStatus(t, "job2", jobs.Failed)
	if n := a.downloads.Load(); n != 5 {
		t.Fatalf("com o circuito aberto o yt-dlp foi chamado: %d vezes, esperado 5", n)
	}
	if cfg.Breaker.State() != breaker.Open {
		t.Fatalf("esperado open, obtido %s", cfg.Breaker.State())
	}

	// Uploads não dependem do YouTube e continuam com o circuito aberto.
	a.upload(t, "up1", "EST001")
	a.esperaStatus(t, "up1", jobs.Ready)

	// O YouTube volta: o half-open testa e fecha o circuito.
	quebrado.Store(false)
	agora.Add(int64(time.Minute))
	a.youtube(t, "job3", "EST001", "ccccccccccc")
	a.esperaStatus(t, "job3", jobs.Ready)
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"open", "half-open", "closed"}; !reflect.DeepEqual(transicoes, want) {
		t.Fatalf("transições %v, esperado %v", transicoes, want)
	}
	if n := a.events.tipos()[jobs.EventTrackFailed]; n != 2 {
		t.Fatalf("%d TRACK_FAILED, esperado 2", n)
	}
}

// ---------------------------------------------------------------- falhas

func TestFalhaTemporariaSeRecupera(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfg := a.config("w1")
	base := cfg.Transcode
	var n atomic.Int32
	cfg.Transcode = func(ctx context.Context, in, out string) (float64, error) {
		if n.Add(1) <= 2 {
			return 0, errors.New("sem espaço em disco")
		}
		return base(ctx, in, out)
	}
	inicia(t, cfg)
	a.upload(t, "job1", "EST001")
	if rec := a.esperaStatus(t, "job1", jobs.Ready); rec.Attempt != 2 {
		t.Fatalf("esperadas 2 novas tentativas: %+v", rec)
	}
	if len(a.queue.Dead()) != 0 {
		t.Fatal("não deveria ir para a DLQ")
	}
}

// Erros que novas tentativas não resolvem vão direto para a DLQ.
func TestErroDefinitivoNaoTentaDeNovo(t *testing.T) {
	casos := map[string]func(*testing.T, *ambiente, *Config) queue.Job{
		"áudio inválido": func(t *testing.T, a *ambiente, cfg *Config) queue.Job {
			cfg.Transcode = func(context.Context, string, string) (float64, error) {
				return 0, fmt.Errorf("%w: xyz", media.ErrInvalidMedia)
			}
			return a.upload(t, "j", "EST001")
		},
		"vídeo privado": func(t *testing.T, a *ambiente, cfg *Config) queue.Job {
			cfg.Download = func(context.Context, string, string) (string, float64, error) {
				return "", 0, fmt.Errorf("%w: xyz", media.ErrVideoUnavailable)
			}
			return a.youtube(t, "j", "EST001", "dQw4w9WgXcQ")
		},
		"original sumiu": func(t *testing.T, a *ambiente, cfg *Config) queue.Job {
			job := queue.Job{ID: "j", Kind: queue.KindUpload, StationID: "EST001", SourceKey: "originals/nada.mp3"}
			a.publica(t, job)
			return job
		},
		"estação apagada": func(t *testing.T, a *ambiente, cfg *Config) queue.Job {
			return a.upload(t, "j", "NAOEXI")
		},
		"mensagem incompleta": func(t *testing.T, a *ambiente, cfg *Config) queue.Job {
			job := queue.Job{ID: "j", StationID: "EST001", Kind: "outro"}
			a.publica(t, job)
			return job
		},
	}
	for nome, prepara := range casos {
		t.Run(nome, func(t *testing.T) {
			a := novoAmbiente(t, "EST001")
			cfg := a.config("w1")
			prepara(t, a, &cfg)
			inicia(t, cfg)
			a.esperaStatus(t, "j", jobs.Failed)
			if len(a.queue.Dead()) != 1 || len(a.queue.Delays()) != 0 {
				t.Fatalf("DLQ=%+v atrasos=%v", a.queue.Dead(), a.queue.Delays())
			}
		})
	}
}

// Vídeos indisponíveis são resposta válida do YouTube e não abrem o circuito.
func TestVideoIndisponivelNaoAbreOCircuito(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfg := a.config("w1")
	cfg.Breaker = NewBreaker(breaker.Config{MaxFailures: 2})
	cfg.Download = func(context.Context, string, string) (string, float64, error) {
		return "", 0, media.ErrVideoUnavailable
	}
	inicia(t, cfg)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("job%d", i)
		a.youtube(t, id, "EST001", fmt.Sprintf("video%06d", i))
		a.esperaStatus(t, id, jobs.Failed)
	}
	if cfg.Breaker.State() != breaker.Closed {
		t.Fatalf("estado %s", cfg.Breaker.State())
	}
}

// Redis fora do ar: o job espera, sem gastar tentativas, e conclui na volta.
func TestRedisForaDoArAdiaSemGastarTentativa(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	a.upload(t, "job1", "EST001")
	a.jobs.SetAvailable(false)
	inicia(t, a.config("w1"))
	espera(t, "vários adiamentos", func() bool { return len(a.queue.Delays()) > 5 })
	a.jobs.SetAvailable(true)
	if rec := a.esperaStatus(t, "job1", jobs.Ready); rec.Attempt != 0 {
		t.Fatalf("gastou tentativas: %+v", rec)
	}
}

// Postgres cai no meio: a conversão é tentada de novo, nada se perde.
func TestPostgresForaDoArTentaDeNovo(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfg := a.config("w1")
	base := cfg.Transcode
	var caiu atomic.Bool
	cfg.Transcode = func(ctx context.Context, in, out string) (float64, error) {
		d, err := base(ctx, in, out)
		if caiu.CompareAndSwap(false, true) {
			a.repo.SetAvailable(false)
			time.AfterFunc(2*time.Millisecond, func() { a.repo.SetAvailable(true) })
		}
		return d, err
	}
	cfg.Retry = queue.RetryPolicy{Delays: []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond}}
	inicia(t, cfg)
	a.upload(t, "job1", "EST001")
	a.esperaStatus(t, "job1", jobs.Ready)
	if repo := a.repositorio(t, "EST001"); len(repo) != 1 {
		t.Fatalf("repositório: %+v", repo)
	}
}

// Parar o worker (deploy) devolve o job em andamento na hora, com o lock solto.
func TestDesligamentoDevolveOJob(t *testing.T) {
	a := novoAmbiente(t, "EST001")
	cfg := a.config("w1")
	cfg.LockTTL = time.Hour
	entrou := make(chan struct{})
	cfg.Transcode = func(ctx context.Context, _, _ string) (float64, error) {
		close(entrou)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	para := inicia(t, cfg)
	a.upload(t, "job1", "EST001")
	<-entrou
	para()
	inicia(t, a.config("w2"))
	if rec := a.esperaStatus(t, "job1", jobs.Ready); rec.Attempt != 0 {
		t.Fatalf("o desligamento gastou tentativa: %+v", rec)
	}
}
