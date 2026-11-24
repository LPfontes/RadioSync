// Package jobs guarda o andamento dos jobs de mídia e os locks que garantem a
// idempotência do worker (ADR-0002). É compartilhado pela Media API e pelo
// Media Worker.
package jobs

import (
	"context"
	"time"

	"radio-backend/internal/model"
	"radio-backend/internal/queue"
)

// Eventos publicados no canal station:{id}:events ao fim de um job.
const (
	EventTrackReady  = "TRACK_READY"
	EventTrackFailed = "TRACK_FAILED"
)

type Status string

const (
	Queued     Status = "queued"
	Processing Status = "processing"
	Retrying   Status = "retrying"
	Ready      Status = "ready"
	Failed     Status = "failed"
)

const (
	// recordTTL é por quanto tempo o andamento de um job pode ser consultado.
	recordTTL = 24 * time.Hour
	// maxPerStation limita a lista de jobs recentes de uma estação.
	maxPerStation = 50
)

// Record é o andamento de um job, como a API devolve ao frontend.
type Record struct {
	ID        string       `json:"jobId"`
	StationID string       `json:"stationId"`
	Kind      queue.Kind   `json:"kind"`
	Title     string       `json:"title,omitempty"`
	Status    Status       `json:"status"`
	Attempt   int          `json:"attempt"`
	Error     string       `json:"error,omitempty"`
	Track     *model.Track `json:"track,omitempty"`
	CreatedAt int64        `json:"createdAt"`
	UpdatedAt int64        `json:"updatedAt"`
}

// NewRecord monta o registro de um job a partir da mensagem da fila, então a
// API e o worker chegam ao mesmo registro sem precisar ler antes de gravar.
func NewRecord(job queue.Job, status Status) Record {
	title := job.Title
	if title == "" {
		title = job.VideoURL
	}
	return Record{
		ID:        job.ID,
		StationID: job.StationID,
		Kind:      job.Kind,
		Title:     title,
		Status:    status,
		Attempt:   job.Attempt,
		CreatedAt: job.EnqueuedAt,
		UpdatedAt: time.Now().UnixMilli(),
	}
}

// Event é a mensagem que chega ao frontend pelo WebSocket.
type Event struct {
	Type  string       `json:"type"`
	JobID string       `json:"jobId"`
	Title string       `json:"title,omitempty"`
	Error string       `json:"error,omitempty"`
	Track *model.Track `json:"track,omitempty"`
}

type Store interface {
	Save(ctx context.Context, rec Record) error
	Delete(ctx context.Context, rec Record) error
	Get(ctx context.Context, jobID string) (Record, bool, error)
	// ListByStation devolve os jobs recentes, do mais novo para o mais antigo.
	ListByStation(ctx context.Context, stationID string) ([]Record, error)
	// Lock tenta pegar o lock; ok=false se outro worker já o tem.
	Lock(ctx context.Context, key string, ttl time.Duration) (lock Lock, ok bool, err error)
}

// Lock expira sozinho se o dono morrer sem renovar.
type Lock interface {
	// Refresh renova o prazo; false se o lock já não é deste dono.
	Refresh(ctx context.Context, ttl time.Duration) (bool, error)
	Release(ctx context.Context) error
}
