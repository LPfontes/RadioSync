package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// Métricas HTTP comuns a todos os serviços (latência, volume e erros por rota).
//
// O rótulo "route" usa o padrão do roteador (ex.: /api/v1/stations/{stationId}),
// nunca a URL real. Com a URL real cada estação criaria uma série nova e a
// cardinalidade do Prometheus cresceria sem limite.
var (
	// HTTPRequestsTotal conta requisições por serviço, método, rota e código.
	// A taxa de erro sai disso: sum(rate(...{code=~"5.."})) / sum(rate(...)).
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "radiosync",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Requisições HTTP atendidas, por serviço, método, rota e código de status",
		},
		[]string{"service", "method", "route", "code"},
	)

	// HTTPRequestDuration alimenta o p95 por rota:
	// histogram_quantile(0.95, sum by (le, route) (rate(..._bucket[5m]))).
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "radiosync",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Latência das requisições HTTP em segundos (exclui upgrades de WebSocket)",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "route"},
	)

	// HTTPRequestsInFlight mostra quantas requisições estão em andamento agora.
	HTTPRequestsInFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "radiosync",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Requisições HTTP em andamento (exclui WebSocket)",
		},
		[]string{"service"},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequestsTotal, HTTPRequestDuration, HTTPRequestsInFlight)
}

// unmatchedRoute é o rótulo das requisições que nenhuma rota atendeu (404 e
// afins). Um único valor fixo evita que varreduras de URL inflem as séries.
const unmatchedRoute = "unmatched"

// HTTPMiddleware mede cada requisição do serviço indicado.
//
// Deve ficar ANTES do Recoverer na cadeia (r.Use), para que um panic
// recuperado como 500 também entre na contagem de erros.
func HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Upgrade de WebSocket: a conexão dura horas e destruiria o p95.
			// Conta só o handshake e deixa a duração de fora. As conexões
			// abertas já têm métrica própria (radiosync_sync_active_connections).
			if isWebSocketUpgrade(r) {
				ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
				next.ServeHTTP(ww, r)
				HTTPRequestsTotal.WithLabelValues(service, r.Method, routePattern(r), statusLabel(ww.Status())).Inc()
				return
			}

			HTTPRequestsInFlight.WithLabelValues(service).Inc()
			defer HTTPRequestsInFlight.WithLabelValues(service).Dec()

			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			route := routePattern(r)
			// Raspagem do Prometheus e healthcheck não são tráfego de usuário.
			if route == "/metrics" || route == "/healthz" {
				return
			}
			HTTPRequestDuration.WithLabelValues(service, r.Method, route).Observe(time.Since(start).Seconds())
			HTTPRequestsTotal.WithLabelValues(service, r.Method, route, statusLabel(ww.Status())).Inc()
		})
	}
}

// routePattern devolve o padrão da rota que atendeu a requisição. Só é
// confiável depois de next.ServeHTTP, quando o chi já preencheu o contexto.
func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return unmatchedRoute
}

// statusLabel converte o status em rótulo. Status 0 significa que o handler
// não escreveu nada; o net/http responde 200 nesse caso.
func statusLabel(status int) string {
	if status == 0 {
		status = http.StatusOK
	}
	return strconv.Itoa(status)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
