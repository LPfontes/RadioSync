package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ActiveConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "radiosync",
			Subsystem: "sync",
			Name:      "active_connections",
			Help:      "Número atual de conexões WebSocket ativas",
		},
		[]string{"role"}, // "dj", "listener"
	)

	MessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "radiosync",
			Subsystem: "sync",
			Name:      "messages_total",
			Help:      "Total de mensagens WebSocket processadas",
		},
		[]string{"direction", "type"}, // direction: "in", "out"
	)

	FanoutLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "radiosync",
			Subsystem: "sync",
			Name:      "fanout_duration_seconds",
			Help:      "Latência de propagação do fan-out via Redis Pub/Sub em segundos",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.5, 1.0},
		},
	)

	ReconnectionsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "radiosync",
			Subsystem: "sync",
			Name:      "reconnections_total",
			Help:      "Total de reconexões atendidas pelo Sync Service",
		},
	)

	DJsOnline = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "radiosync",
			Subsystem: "sync",
			Name:      "djs_online",
			Help:      "Número de DJs ativos nesta réplica",
		},
	)
)

func init() {
	prometheus.MustRegister(ActiveConnections)
	prometheus.MustRegister(MessagesTotal)
	prometheus.MustRegister(FanoutLatency)
	prometheus.MustRegister(ReconnectionsTotal)
	prometheus.MustRegister(DJsOnline)
}

// Handler retorna o endpoint HTTP para raspagem do Prometheus (/metrics).
func Handler() http.Handler {
	return promhttp.Handler()
}

// ClientConnected incrementa métricas de conexão.
func ClientConnected(role string) {
	ActiveConnections.WithLabelValues(role).Inc()
	if role == "dj" {
		DJsOnline.Inc()
	}
}

// ClientDisconnected decrementa métricas de conexão.
func ClientDisconnected(role string) {
	ActiveConnections.WithLabelValues(role).Dec()
	if role == "dj" {
		DJsOnline.Dec()
	}
}

// MessageReceived incrementa contador de mensagens recebidas.
func MessageReceived(msgType string) {
	MessagesTotal.WithLabelValues("in", msgType).Inc()
}

// MessageSent incrementa contador de mensagens enviadas.
func MessageSent(msgType string) {
	MessagesTotal.WithLabelValues("out", msgType).Inc()
}

// RecordFanoutLatency registra o tempo gasto no fan-out entre réplicas.
func RecordFanoutLatency(d time.Duration) {
	FanoutLatency.Observe(d.Seconds())
}

// ClientReconnected contabiliza reconexões com sucesso.
func ClientReconnected() {
	ReconnectionsTotal.Inc()
}
