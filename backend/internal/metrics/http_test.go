package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// novoRoteador monta um roteador como o dos serviços reais: o middleware de
// métricas vem antes do Recoverer.
func novoRoteador(service string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(HTTPMiddleware(service))
	r.Use(chimw.Recoverer)

	r.Get("/api/v1/stations/{stationId}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "falhou", http.StatusInternalServerError)
	})
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("erro inesperado")
	})
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	})
	r.Handle("/metrics", Handler())
	return r
}

func requisitar(r http.Handler, method, path string, headers map[string]string) int {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

// contador lê o valor de requests_total para a combinação de rótulos.
func contador(t *testing.T, service, method, route, code string) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := HTTPRequestsTotal.WithLabelValues(service, method, route, code).Write(m); err != nil {
		t.Fatalf("lendo contador: %v", err)
	}
	return m.GetCounter().GetValue()
}

// amostras lê quantas observações o histograma de latência tem para a rota.
func amostras(t *testing.T, service, method, route string) uint64 {
	t.Helper()
	obs, err := HTTPRequestDuration.GetMetricWithLabelValues(service, method, route)
	if err != nil {
		t.Fatalf("lendo histograma: %v", err)
	}
	m := &dto.Metric{}
	if err := obs.(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("lendo histograma: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestHTTPMiddleware_UsaPadraoDaRotaNaoAURL(t *testing.T) {
	r := novoRoteador("teste-rota")

	for _, id := range []string{"AAA111", "BBB222", "CCC333"} {
		if code := requisitar(r, "GET", "/api/v1/stations/"+id, nil); code != 200 {
			t.Fatalf("status = %d, quer 200", code)
		}
	}

	// As três estações caem na MESMA série. Se o rótulo fosse a URL real
	// seriam três séries e a cardinalidade cresceria a cada estação nova.
	if got := contador(t, "teste-rota", "GET", "/api/v1/stations/{stationId}", "200"); got != 3 {
		t.Errorf("requests_total na rota padrão = %v, quer 3", got)
	}
	if got := amostras(t, "teste-rota", "GET", "/api/v1/stations/{stationId}"); got != 3 {
		t.Errorf("amostras de latência = %d, quer 3", got)
	}
}

func TestHTTPMiddleware_ContaErrosPorCodigo(t *testing.T) {
	r := novoRoteador("teste-erro")

	requisitar(r, "GET", "/boom", nil)
	requisitar(r, "GET", "/boom", nil)

	if got := contador(t, "teste-erro", "GET", "/boom", "500"); got != 2 {
		t.Errorf("500 em /boom = %v, quer 2", got)
	}
	if got := contador(t, "teste-erro", "GET", "/boom", "200"); got != 0 {
		t.Errorf("200 em /boom = %v, quer 0", got)
	}
}

func TestHTTPMiddleware_PanicRecuperadoVira500(t *testing.T) {
	r := novoRoteador("teste-panic")

	if code := requisitar(r, "GET", "/panic", nil); code != 500 {
		t.Fatalf("status = %d, quer 500", code)
	}
	// O middleware está antes do Recoverer, então enxerga o 500 resultante.
	if got := contador(t, "teste-panic", "GET", "/panic", "500"); got != 1 {
		t.Errorf("500 em /panic = %v, quer 1", got)
	}
}

func TestHTTPMiddleware_RotaInexistenteFicaEmUnmatched(t *testing.T) {
	r := novoRoteador("teste-404")

	// Varredura de URLs aleatórias não pode criar uma série por URL.
	for _, p := range []string{"/wp-admin", "/.env", "/qualquer/coisa"} {
		if code := requisitar(r, "GET", p, nil); code != 404 {
			t.Fatalf("%s: status = %d, quer 404", p, code)
		}
	}
	if got := contador(t, "teste-404", "GET", unmatchedRoute, "404"); got != 3 {
		t.Errorf("404 em unmatched = %v, quer 3", got)
	}
}

func TestHTTPMiddleware_WebSocketNaoEntraNoHistograma(t *testing.T) {
	r := novoRoteador("teste-ws")

	requisitar(r, "GET", "/ws", map[string]string{"Upgrade": "websocket"})

	// O handshake é contado, mas a duração não vai para o histograma: uma
	// conexão de horas distorceria o p95 de todas as rotas.
	if got := contador(t, "teste-ws", "GET", "/ws", "101"); got != 1 {
		t.Errorf("101 em /ws = %v, quer 1", got)
	}
	if got := amostras(t, "teste-ws", "GET", "/ws"); got != 0 {
		t.Errorf("amostras de latência do WebSocket = %d, quer 0", got)
	}
}

func TestHTTPMiddleware_IgnoraMetricsEHealthz(t *testing.T) {
	r := novoRoteador("teste-ignora")

	requisitar(r, "GET", "/metrics", nil)

	if got := contador(t, "teste-ignora", "GET", "/metrics", "200"); got != 0 {
		t.Errorf("a raspagem do Prometheus foi contada (%v), mas não deveria", got)
	}
}

func TestHTTPMiddleware_InFlightVoltaAZero(t *testing.T) {
	r := novoRoteador("teste-inflight")

	requisitar(r, "GET", "/api/v1/stations/XYZ", nil)

	m := &dto.Metric{}
	if err := HTTPRequestsInFlight.WithLabelValues("teste-inflight").Write(m); err != nil {
		t.Fatal(err)
	}
	if got := m.GetGauge().GetValue(); got != 0 {
		t.Errorf("in_flight depois da resposta = %v, quer 0", got)
	}
}

func TestHTTPMiddleware_ExpostoEmMetrics(t *testing.T) {
	r := novoRoteador("teste-expo")
	requisitar(r, "GET", "/api/v1/stations/ABC", nil)

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	achou := map[string]bool{}
	for _, mf := range mfs {
		achou[mf.GetName()] = true
	}
	for _, nome := range []string{
		"radiosync_http_requests_total",
		"radiosync_http_request_duration_seconds",
	} {
		if !achou[nome] {
			t.Errorf("métrica %s não aparece no registro padrão", nome)
		}
	}
}

// O gorilla/websocket faz Hijack e escreve o 101 na conexão crua, sem passar
// pelo WriteHeader. Por isso o handshake precisa ser contado no Hijack: se fosse
// quando o handler retorna, só apareceria ao FECHAR a conexão, e com código 200.
func TestHTTPMiddleware_WebSocketReal_Conta101NoHandshake(t *testing.T) {
	r := chi.NewRouter()
	r.Use(HTTPMiddleware("teste-wsreal"))
	fechar := make(chan struct{})
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	r.Get("/ws/stations/{stationId}", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		<-fechar // o handler só retorna quando a conexão acaba
		c.Close()
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	defer close(fechar)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/stations/ABC", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// conexão aberta e handler ainda preso: o 101 já tem de estar contado
	prazo := time.Now().Add(2 * time.Second)
	for contador(t, "teste-wsreal", "GET", "/ws/stations/{stationId}", "101") < 1 {
		if time.Now().After(prazo) {
			t.Fatal("o handshake não foi contado como 101 enquanto a conexão estava aberta")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := contador(t, "teste-wsreal", "GET", "/ws/stations/{stationId}", "200"); got != 0 {
		t.Errorf("o handshake foi contado como 200 (%v vezes)", got)
	}
}

// Upgrade recusado (ex.: Origin inválida) não faz Hijack: vale o status real.
func TestHTTPMiddleware_WebSocketRecusado_ContaOStatusReal(t *testing.T) {
	r := chi.NewRouter()
	r.Use(HTTPMiddleware("teste-wsnegado"))
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return false }}
	r.Get("/ws/stations/{stationId}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = up.Upgrade(w, r, nil) // responde 403 sozinho
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/stations/ABC",
		http.Header{"Origin": {"http://evil.example"}})
	if err == nil {
		t.Fatal("o upgrade deveria ter sido recusado")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("resposta = %v, quer 403", resp)
	}
	if got := contador(t, "teste-wsnegado", "GET", "/ws/stations/{stationId}", "403"); got != 1 {
		t.Errorf("403 contado %v vezes, quer 1", got)
	}
}
