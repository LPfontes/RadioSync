// Package mediaworker consome a fila media.transcode (ADR-0002): converte
// uploads com FFmpeg, baixa do YouTube com yt-dlp e publica a faixa pronta.
package mediaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"radio-backend/internal/breaker"
	"radio-backend/internal/jobs"
	"radio-backend/internal/media"
	"radio-backend/internal/metrics"
	"radio-backend/internal/model"
	"radio-backend/internal/queue"
	"radio-backend/internal/storage"
	"radio-backend/internal/syncservice"
)

// Transcoder converte um arquivo para Opus e devolve a duração em segundos.
type Transcoder func(ctx context.Context, input, output string) (float64, error)

// Downloader baixa o áudio de um vídeo em Opus.
type Downloader func(ctx context.Context, videoURL, output string) (title string, duration float64, err error)

// EventPublisher publica eventos no canal da estação (Redis Pub/Sub).
type EventPublisher interface {
	PublishEvent(ctx context.Context, stationID string, event *syncservice.RedisEvent) error
}

type Config struct {
	ID         string
	Queue      queue.Queue
	Jobs       jobs.Store
	Repo       storage.Repository
	Objects    storage.ObjectStore
	Events     EventPublisher
	Transcode  Transcoder
	Download   Downloader
	Breaker    *breaker.Breaker
	Retry      queue.RetryPolicy
	JobTimeout time.Duration
	// LockTTL é em quanto tempo o lock de um worker morto expira.
	LockTTL time.Duration
	// Concurrency é o máximo de jobs ao mesmo tempo nesta réplica (bulkhead).
	Concurrency int
}

type Worker struct {
	cfg Config
}

var (
	// errLockLost: outro worker pode ter assumido o job; este precisa parar.
	errLockLost = errors.New("lock do job perdido")
	// errStationGone e errSourceGone não melhoram com novas tentativas.
	errStationGone = errors.New("a estação não existe mais")
	errSourceGone  = errors.New("o arquivo enviado não foi encontrado")
)

func New(cfg Config) *Worker {
	if len(cfg.Retry.Delays) == 0 {
		cfg.Retry = queue.DefaultRetryPolicy()
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 10 * time.Minute
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = 15 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Breaker == nil {
		cfg.Breaker = NewBreaker(breaker.Config{})
	}
	if cfg.Transcode == nil {
		cfg.Transcode = func(ctx context.Context, input, output string) (float64, error) {
			if err := media.ConvertToOpus(ctx, input, output); err != nil {
				return 0, err
			}
			return media.GetDuration(ctx, output)
		}
	}
	if cfg.Download == nil {
		cfg.Download = media.DownloadYouTubeAudio
	}
	return &Worker{cfg: cfg}
}

// NewBreaker cria o circuit breaker do YouTube. Um vídeo privado ou removido é
// uma resposta válida do YouTube e o desligamento do worker não diz nada sobre
// ele: nenhum dos dois conta como falha.
func NewBreaker(cfg breaker.Config) *breaker.Breaker {
	cfg.Classify = func(err error) breaker.Outcome {
		switch {
		case err == nil, errors.Is(err, media.ErrVideoUnavailable):
			return breaker.Success
		case errors.Is(err, context.Canceled):
			return breaker.Ignored
		}
		return breaker.Failure
	}
	next := cfg.OnStateChange
	cfg.OnStateChange = func(from, to breaker.State) {
		metrics.MediaBreakerState.Set(float64(to))
		log.Printf("[worker] circuit breaker do YouTube: %s -> %s", from, to)
		if next != nil {
			next(from, to)
		}
	}
	return breaker.New(cfg)
}

// Run consome até ctx terminar.
func (w *Worker) Run(ctx context.Context) error {
	return w.cfg.Queue.Consume(ctx, w.cfg.Concurrency, w.handle)
}

func isPermanent(err error) bool {
	return errors.Is(err, errStationGone) || errors.Is(err, errSourceGone) ||
		errors.Is(err, media.ErrInvalidMedia) || errors.Is(err, media.ErrVideoUnavailable) ||
		errors.Is(err, media.ErrInvalidYouTubeURL)
}

// userMessage traduz o erro para o DJ, sem expor o log do FFmpeg/yt-dlp.
func userMessage(kind queue.Kind, err error) string {
	for _, known := range []error{errStationGone, errSourceGone, media.ErrInvalidMedia, media.ErrVideoUnavailable, media.ErrInvalidYouTubeURL} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	if kind == queue.KindYouTube {
		return "não foi possível baixar do YouTube agora; tente de novo mais tarde"
	}
	return "não foi possível converter o arquivo"
}

// lockKey: um lock por job e, no YouTube, por vídeo, para que duas estações que
// pedem o mesmo vídeo ao mesmo tempo não o baixem duas vezes.
func lockKey(job queue.Job) string {
	if job.Kind == queue.KindYouTube {
		return "youtube:" + job.VideoID
	}
	return "job:" + job.ID
}

func (w *Worker) handle(ctx context.Context, d *queue.Delivery) {
	job := d.Job
	start := time.Now()
	metrics.MediaJobsInProgress.Inc()
	defer metrics.MediaJobsInProgress.Dec()

	result := w.handleJob(ctx, d)
	metrics.MediaAttempt(string(job.Kind), result, time.Since(start))
	log.Printf("[worker %s] job %s (%s) estação %s tentativa %d: %s em %v",
		w.cfg.ID, job.ID, job.Kind, job.StationID, job.Attempt+1, result, time.Since(start).Round(time.Millisecond))
}

// handleJob processa a entrega e devolve o resultado para as métricas. A
// mensagem só sai da fila (Ack) depois que a faixa está salva, ou depois que
// ela foi regravada com sucesso na fila de retry ou na DLQ.
func (w *Worker) handleJob(ctx context.Context, d *queue.Delivery) string {
	job := d.Job
	if err := job.Validate(); err != nil {
		return w.fail(ctx, d, err)
	}

	lock, ok, err := w.cfg.Jobs.Lock(ctx, lockKey(job), w.cfg.LockTTL)
	if err != nil || !ok {
		// Redis fora do ar ou outro worker no mesmo job: volta depois, sem
		// gastar uma tentativa.
		return w.postpone(ctx, d, err)
	}
	workCtx, stop := w.keepLock(ctx, lock)
	defer func() {
		stop()
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = lock.Release(releaseCtx)
	}()

	w.save(ctx, jobs.NewRecord(job, jobs.Processing))

	jobCtx, cancel := context.WithTimeout(workCtx, w.cfg.JobTimeout)
	track, cached, err := w.process(jobCtx, job)
	cancel()

	switch {
	case err == nil:
		return w.succeed(ctx, d, track, cached)
	case ctx.Err() != nil:
		// O worker está parando ou perdeu o broker: outro assume.
		return w.requeue(d)
	case errors.Is(context.Cause(workCtx), errLockLost):
		return w.postpone(ctx, d, errLockLost)
	case isPermanent(err):
		return w.fail(ctx, d, err)
	}
	return w.retry(ctx, d, err)
}

// keepLock renova o lock até stop. Se o lock for perdido, cancela o contexto:
// dois workers nunca publicam o mesmo job.
func (w *Worker) keepLock(ctx context.Context, lock jobs.Lock) (context.Context, func()) {
	lockCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(w.cfg.LockTTL / 3)
		defer ticker.Stop()
		lastOK := time.Now()
		for {
			select {
			case <-lockCtx.Done():
				return
			case <-ticker.C:
			}
			held, err := lock.Refresh(lockCtx, w.cfg.LockTTL)
			switch {
			case err == nil && held:
				lastOK = time.Now()
			case err == nil, time.Since(lastOK) >= w.cfg.LockTTL:
				cancel(errLockLost)
				return
			}
		}
	}()
	return lockCtx, func() {
		cancel(nil)
		<-done
	}
}

// ---------------------------------------------------------------- processamento

func (w *Worker) process(ctx context.Context, job queue.Job) (track model.Track, cached bool, err error) {
	// Antes de gastar CPU: a estação ainda existe?
	if _, err := w.cfg.Repo.GetStation(ctx, job.StationID); err != nil {
		return model.Track{}, false, stationError(err)
	}
	if job.Kind == queue.KindYouTube {
		return w.processYouTube(ctx, job)
	}
	return w.processUpload(ctx, job)
}

func stationError(err error) error {
	if errors.Is(err, storage.ErrStationNotFound) {
		return errStationGone
	}
	return err
}

func (w *Worker) processUpload(ctx context.Context, job queue.Job) (model.Track, bool, error) {
	// Reentrega de um job que já tinha terminado: não converte de novo.
	track, ok, err := w.cfg.Repo.GetTrack(ctx, job.ID)
	if err != nil {
		return model.Track{}, false, err
	}
	if ok {
		exists, err := w.cfg.Objects.Exists(ctx, track.Filename)
		if err != nil {
			return model.Track{}, false, err
		}
		if exists {
			return track, true, w.addToStation(ctx, job.StationID, track)
		}
	}

	dir, err := os.MkdirTemp("", "media-job-")
	if err != nil {
		return model.Track{}, false, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "original"+filepath.Ext(job.SourceKey))
	if err := w.fetch(ctx, job.SourceKey, input); err != nil {
		return model.Track{}, false, err
	}
	output := filepath.Join(dir, job.ID+".opus")
	duration, err := w.cfg.Transcode(ctx, input, output)
	if err != nil {
		return model.Track{}, false, err
	}

	track = newTrack(job.ID, job, job.Title, duration)
	if err := w.publishTrack(ctx, job.StationID, track, output); err != nil {
		return model.Track{}, false, err
	}
	if err := w.cfg.Objects.Delete(ctx, job.SourceKey); err != nil {
		log.Printf("[worker] não foi possível apagar o original %s: %v", job.SourceKey, err)
	}
	return track, false, nil
}

func (w *Worker) processYouTube(ctx context.Context, job queue.Job) (model.Track, bool, error) {
	track, known, err := w.cfg.Repo.GetYouTubeCache(ctx, job.VideoID)
	if err != nil {
		return model.Track{}, false, err
	}
	trackID := job.ID
	if known {
		exists, err := w.cfg.Objects.Exists(ctx, track.Filename)
		if err != nil {
			return model.Track{}, false, err
		}
		if exists {
			return track, true, w.addToStation(ctx, job.StationID, track)
		}
		// O áudio sumiu: baixa de novo no mesmo ID, para consertar a faixa em
		// todas as estações que já a usam.
		trackID = track.ID
	}

	dir, err := os.MkdirTemp("", "media-job-")
	if err != nil {
		return model.Track{}, false, err
	}
	defer os.RemoveAll(dir)

	output := filepath.Join(dir, trackID+".opus")
	var title string
	var duration float64
	err = w.cfg.Breaker.Execute(func() error {
		var err error
		title, duration, err = w.cfg.Download(ctx, job.VideoURL, output)
		return err
	})
	if err != nil {
		return model.Track{}, false, err
	}

	if known {
		track.Filename, track.URL, track.Duration = trackID+".opus", "/musicas/"+trackID+".opus", duration
	} else {
		track = newTrack(trackID, job, title, duration)
	}
	if err := w.publishTrack(ctx, job.StationID, track, output); err != nil {
		return model.Track{}, false, err
	}
	if err := w.cfg.Repo.SaveYouTubeCache(ctx, job.VideoID, trackID); err != nil {
		return model.Track{}, false, err
	}
	return track, false, nil
}

func newTrack(id string, job queue.Job, title string, duration float64) model.Track {
	return model.Track{
		ID:       id,
		Title:    title,
		Filename: id + ".opus",
		URL:      "/musicas/" + id + ".opus",
		Duration: duration,
		Category: job.Category,
		Theme:    job.Theme,
		CoverURL: job.CoverURL,
	}
}

func (w *Worker) fetch(ctx context.Context, key, path string) error {
	src, _, err := w.cfg.Objects.Get(ctx, key)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return errSourceGone
	}
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(path) // #nosec G304 -- dentro do diretório temporário do job
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	return err
}

// publishTrack grava o .opus no ObjectStore e só depois a faixa no repositório
// da estação: uma faixa visível sempre tem áudio.
func (w *Worker) publishTrack(ctx context.Context, stationID string, track model.Track, path string) error {
	f, err := os.Open(path) // #nosec G304 -- dentro do diretório temporário do job
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("a conversão gerou um arquivo vazio")
	}
	if err := w.cfg.Objects.Put(ctx, track.Filename, f, info.Size(), "audio/ogg"); err != nil {
		return err
	}
	return w.addToStation(ctx, stationID, track)
}

func (w *Worker) addToStation(ctx context.Context, stationID string, track model.Track) error {
	return stationError(w.cfg.Repo.AddTrackToStation(ctx, stationID, track))
}

// ---------------------------------------------------------------- desfechos

func (w *Worker) succeed(ctx context.Context, d *queue.Delivery, track model.Track, cached bool) string {
	job := d.Job
	rec := jobs.NewRecord(job, jobs.Ready)
	rec.Title, rec.Track = track.Title, &track
	// Sem esse registro o DJ veria "processando…" para sempre. A faixa já está
	// salva, então refazer o job só repete o aviso.
	if err := w.cfg.Jobs.Save(ctx, rec); err != nil {
		return w.postpone(ctx, d, err)
	}
	w.notify(ctx, job.StationID, jobs.Event{Type: jobs.EventTrackReady, JobID: job.ID, Title: track.Title, Track: &track})
	metrics.MediaJobs.WithLabelValues(string(job.Kind), "ok").Inc()
	w.ack(d)
	if cached {
		return "cached"
	}
	return "ok"
}

// retry agenda uma nova tentativa com backoff ou, esgotadas, vai para a DLQ.
func (w *Worker) retry(ctx context.Context, d *queue.Delivery, cause error) string {
	delay, ok := w.cfg.Retry.Next(d.Job.Attempt)
	if !ok {
		return w.fail(ctx, d, cause)
	}
	next := d.Job
	next.Attempt++
	if err := w.cfg.Queue.PublishDelayed(ctx, next, delay); err != nil {
		return w.requeue(d)
	}
	log.Printf("[worker] job %s falhou (%v); nova tentativa %d/%d em %v",
		next.ID, cause, next.Attempt, len(w.cfg.Retry.Delays), delay)
	w.save(ctx, jobs.NewRecord(next, jobs.Retrying))
	w.ack(d)
	return "retry"
}

func (w *Worker) fail(ctx context.Context, d *queue.Delivery, cause error) string {
	job := d.Job
	if err := w.cfg.Queue.DeadLetter(ctx, job, cause.Error()); err != nil {
		return w.requeue(d)
	}
	log.Printf("[worker] job %s foi para a DLQ depois de %d tentativas: %v", job.ID, job.Attempt+1, cause)
	if job.StationID != "" {
		rec := jobs.NewRecord(job, jobs.Failed)
		rec.Error = userMessage(job.Kind, cause)
		w.save(ctx, rec)
		w.notify(ctx, job.StationID, jobs.Event{Type: jobs.EventTrackFailed, JobID: job.ID, Title: rec.Title, Error: rec.Error})
	}
	metrics.MediaJobs.WithLabelValues(string(job.Kind), "failed").Inc()
	w.ack(d)
	return "dead"
}

// postpone devolve o job depois do menor atraso, sem gastar uma tentativa.
func (w *Worker) postpone(ctx context.Context, d *queue.Delivery, cause error) string {
	if err := w.cfg.Queue.PublishDelayed(ctx, d.Job, w.cfg.Retry.Delays[0]); err != nil {
		return w.requeue(d)
	}
	if cause != nil {
		log.Printf("[worker] job %s adiado: %v", d.Job.ID, cause)
	}
	w.ack(d)
	return "postponed"
}

func (w *Worker) requeue(d *queue.Delivery) string {
	if err := d.Requeue(); err != nil {
		log.Printf("[worker] job %s: %v (a fila devolve sozinha)", d.Job.ID, err)
	}
	return "requeued"
}

func (w *Worker) ack(d *queue.Delivery) {
	if err := d.Ack(); err != nil {
		// A mensagem será reentregue; o processamento é idempotente.
		log.Printf("[worker] ack do job %s: %v", d.Job.ID, err)
	}
}

func (w *Worker) save(ctx context.Context, rec jobs.Record) {
	if err := w.cfg.Jobs.Save(ctx, rec); err != nil {
		log.Printf("[worker] não foi possível salvar o andamento do job %s: %v", rec.ID, err)
	}
}

// notify é melhor esforço: o Pub/Sub não guarda mensagens. O frontend também
// consulta o andamento pela API, que lê o registro já salvo.
func (w *Worker) notify(ctx context.Context, stationID string, ev jobs.Event) {
	payload, err := json.Marshal(ev)
	if err == nil {
		err = w.cfg.Events.PublishEvent(ctx, stationID, &syncservice.RedisEvent{
			SenderID:  w.cfg.ID,
			StationID: stationID,
			Type:      ev.Type,
			Payload:   payload,
		})
	}
	if err != nil {
		log.Printf("[worker] não foi possível publicar %s do job %s: %v", ev.Type, ev.JobID, err)
	}
}
