// Package queue define o contrato da fila de transcodificação (ADR-0002) e suas
// implementações: RabbitMQ (produção) e memória (testes).
package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Nomes das filas no RabbitMQ.
const (
	TranscodeQueue  = "media.transcode"
	DeadLetterQueue = "media.transcode.dlq"
	// RetryQueuePrefix + atraso em ms: uma fila por atraso, sem consumidor. A
	// mensagem expira (TTL) e volta sozinha para TranscodeQueue.
	RetryQueuePrefix = "media.transcode.retry."
)

// ErrUnavailable indica que o broker não confirmou a operação.
var ErrUnavailable = errors.New("fila indisponível")

type Kind string

const (
	KindUpload  Kind = "upload"
	KindYouTube Kind = "youtube"
)

// Job é a mensagem da fila media.transcode.
type Job struct {
	ID        string `json:"jobId"`
	Kind      Kind   `json:"kind"`
	StationID string `json:"stationId"`
	Title     string `json:"title,omitempty"`
	Category  string `json:"category,omitempty"`
	Theme     string `json:"theme,omitempty"`
	CoverURL  string `json:"coverUrl,omitempty"`

	// SourceKey é a chave do arquivo original no ObjectStore (upload).
	SourceKey string `json:"sourceKey,omitempty"`
	// VideoURL e VideoID identificam o vídeo (youtube).
	VideoURL string `json:"videoUrl,omitempty"`
	VideoID  string `json:"videoId,omitempty"`

	// Attempt é o número de novas tentativas já feitas (0 na primeira vez).
	Attempt    int   `json:"attempt"`
	EnqueuedAt int64 `json:"enqueuedAt"`

	// Trace leva o contexto do trace (traceparent) do produtor até o worker. Vai
	// no corpo, e não em cabeçalho AMQP, para sobreviver a retry e DLQ, que
	// republicam o Job, e para funcionar igual com a fila em memória.
	Trace map[string]string `json:"trace,omitempty"`
}

// Validate recusa mensagens que nenhuma nova tentativa faria funcionar.
func (j Job) Validate() error {
	if j.ID == "" || j.StationID == "" {
		return errors.New("job sem jobId ou stationId")
	}
	switch j.Kind {
	case KindUpload:
		if j.SourceKey == "" {
			return errors.New("job de upload sem sourceKey")
		}
	case KindYouTube:
		if j.VideoURL == "" || j.VideoID == "" {
			return errors.New("job do YouTube sem videoUrl ou videoId")
		}
	default:
		return fmt.Errorf("tipo de job desconhecido: %q", j.Kind)
	}
	return nil
}

// Delivery é uma mensagem entregue e ainda não confirmada (ack manual): se o
// worker morrer antes do Ack, a fila entrega a mensagem a outro worker.
type Delivery struct {
	Job         Job
	Redelivered bool

	ack  func() error
	nack func() error
}

// Ack remove a mensagem da fila.
func (d *Delivery) Ack() error { return d.ack() }

// Requeue devolve a mensagem para a fila, para outro worker.
func (d *Delivery) Requeue() error { return d.nack() }

// Handler processa uma entrega e precisa encerrá-la com Ack ou Requeue. O ctx é
// cancelado quando o consumidor para ou perde a conexão com o broker.
type Handler func(ctx context.Context, d *Delivery)

// Publisher é o que a Media API usa.
type Publisher interface {
	// Publish só retorna nil depois que o broker confirmou o recebimento.
	Publish(ctx context.Context, job Job) error
}

// Queue é o que o worker usa.
type Queue interface {
	Publisher
	// PublishDelayed agenda o job para voltar à fila depois de delay.
	PublishDelayed(ctx context.Context, job Job, delay time.Duration) error
	// DeadLetter manda o job para a DLQ com o motivo.
	DeadLetter(ctx context.Context, job Job, reason string) error
	// Consume bloqueia até ctx terminar, com no máximo prefetch entregas em
	// andamento (bulkhead), e reconecta se o broker cair.
	Consume(ctx context.Context, prefetch int, handle Handler) error
}

// ---------------------------------------------------------------- retry

// RetryPolicy define os atrasos entre tentativas (backoff exponencial).
type RetryPolicy struct {
	Delays []time.Duration
}

// DefaultRetryPolicy: 3 novas tentativas, depois DLQ (ADR-0002).
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{Delays: []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}}
}

// Next devolve o atraso da próxima tentativa, ou false se acabaram.
func (p RetryPolicy) Next(attempt int) (time.Duration, bool) {
	if attempt < 0 || attempt >= len(p.Delays) {
		return 0, false
	}
	return p.Delays[attempt], true
}

// ParseRetryPolicy lê uma lista como "5s,30s,2m". Vazio devolve o padrão.
func ParseRetryPolicy(s string) (RetryPolicy, error) {
	if strings.TrimSpace(s) == "" {
		return DefaultRetryPolicy(), nil
	}
	var p RetryPolicy
	for _, part := range strings.Split(s, ",") {
		d, err := time.ParseDuration(strings.TrimSpace(part))
		if err != nil || d < time.Millisecond {
			return RetryPolicy{}, fmt.Errorf("atraso de retry inválido: %q", part)
		}
		p.Delays = append(p.Delays, d)
	}
	return p, nil
}
