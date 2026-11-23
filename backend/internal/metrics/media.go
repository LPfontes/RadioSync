package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Métricas da frente de mídia (ADR-0002).
var (
	MediaQueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "queue_depth",
			Help:      "Mensagens prontas em cada fila do RabbitMQ (inclui retry e DLQ)",
		},
		[]string{"queue"},
	)

	MediaJobsEnqueued = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "jobs_enqueued_total",
			Help:      "Pedidos recebidos pela Media API",
		},
		[]string{"kind", "result"}, // result: "ok", "error"
	)

	MediaJobs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "jobs_total",
			Help:      "Jobs encerrados pelo worker",
		},
		[]string{"kind", "result"}, // result: "ok", "failed"
	)

	MediaAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "job_attempts_total",
			Help:      "Tentativas de processamento, por resultado",
		},
		// result: "ok", "cached", "retry", "dead", "requeued", "postponed"
		[]string{"kind", "result"},
	)

	MediaJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "job_duration_seconds",
			Help:      "Duração de cada tentativa de processamento",
			Buckets:   []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"kind"},
	)

	MediaJobsInProgress = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "jobs_in_progress",
			Help:      "Jobs em processamento nesta réplica",
		},
	)

	MediaBreakerState = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "radiosync",
			Subsystem: "media",
			Name:      "breaker_state",
			Help:      "Circuit breaker do YouTube nesta réplica: 0 fechado, 1 half-open, 2 aberto",
		},
	)
)

func init() {
	prometheus.MustRegister(MediaQueueDepth, MediaJobsEnqueued, MediaJobs, MediaAttempts,
		MediaJobDuration, MediaJobsInProgress, MediaBreakerState)
}

// MediaAttempt registra o resultado e a duração de uma tentativa.
func MediaAttempt(kind, result string, d time.Duration) {
	MediaAttempts.WithLabelValues(kind, result).Inc()
	MediaJobDuration.WithLabelValues(kind).Observe(d.Seconds())
}
