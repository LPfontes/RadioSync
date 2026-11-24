package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// exportadorDeTeste instala um provedor que guarda os spans em memória.
func exportadorDeTeste(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return exp
}

func roteador() *chi.Mux {
	r := chi.NewRouter()
	r.Use(HTTPMiddleware("teste"))
	r.Use(chimw.Recoverer)
	r.Get("/api/v1/stations/{stationId}", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", http.StatusInternalServerError) })
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) { panic("falha") })
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("m")) })
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusSwitchingProtocols) })
	return r
}

func chamar(r http.Handler, path string, cab map[string]string) int {
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestHTTPMiddleware_NomeDoSpanUsaAPadraoDaRota(t *testing.T) {
	exp := exportadorDeTeste(t)
	r := roteador()

	chamar(r, "/api/v1/stations/ABC123", nil)
	chamar(r, "/api/v1/stations/ZZZ999", nil)

	spans := exp.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("%d spans, quer 2", len(spans))
	}
	for _, s := range spans {
		// duas estações diferentes, o MESMO nome: sem explosão de nomes no Jaeger
		if s.Name != "GET /api/v1/stations/{stationId}" {
			t.Errorf("nome do span = %q", s.Name)
		}
	}
}

func TestHTTPMiddleware_ErroDoServidorMarcaOSpan(t *testing.T) {
	exp := exportadorDeTeste(t)
	chamar(roteador(), "/boom", nil)

	s := exp.GetSpans()[0]
	if s.Status.Code != codes.Error {
		t.Fatalf("status do span = %v, quer Error (HTTP 500)", s.Status.Code)
	}
}

func TestHTTPMiddleware_PanicRecuperadoMarcaOSpan(t *testing.T) {
	exp := exportadorDeTeste(t)
	if code := chamar(roteador(), "/panic", nil); code != 500 {
		t.Fatalf("status = %d, quer 500", code)
	}
	if s := exp.GetSpans()[0]; s.Status.Code != codes.Error {
		t.Fatalf("status do span = %v, quer Error", s.Status.Code)
	}
}

func TestHTTPMiddleware_NaoCriaSpanParaMetricsHealthzEWebSocket(t *testing.T) {
	exp := exportadorDeTeste(t)
	r := roteador()

	chamar(r, "/metrics", nil)
	chamar(r, "/healthz", nil)
	chamar(r, "/ws", map[string]string{"Upgrade": "websocket"})

	if n := len(exp.GetSpans()); n != 0 {
		t.Fatalf("%d spans criados para tráfego que não deveria ser rastreado", n)
	}
}

func TestHTTPMiddleware_ContinuaOTraceDoGateway(t *testing.T) {
	exp := exportadorDeTeste(t)

	const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	chamar(roteador(), "/api/v1/stations/ABC", map[string]string{"traceparent": traceparent})

	s := exp.GetSpans()[0]
	if got := s.SpanContext.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace id = %s, o span deveria continuar o trace que o gateway começou", got)
	}
	if got := s.Parent.SpanID().String(); got != "00f067aa0ba902b7" {
		t.Errorf("pai = %s, quer o span do gateway", got)
	}
}

// Inject/Extract é como o trace atravessa a fila e o Redis: o consumidor tem de
// ficar no mesmo trace do produtor, como filho dele.
func TestInjectExtract_ProdutorEConsumidorNoMesmoTrace(t *testing.T) {
	exp := exportadorDeTeste(t)

	ctxProd, prod := StartProducer(context.Background(), "queue.publish")
	carrier := Inject(ctxProd)
	prod.End()
	if len(carrier) == 0 || carrier["traceparent"] == "" {
		t.Fatalf("Inject não gerou traceparent: %v", carrier)
	}

	// o consumidor roda "em outro processo": começa de um contexto vazio
	_, cons := StartConsumer(context.Background(), "media.job", carrier)
	cons.End()

	spans := exp.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("%d spans, quer 2", len(spans))
	}
	var produtor, consumidor tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "queue.publish" {
			produtor = s
		} else {
			consumidor = s
		}
	}
	if produtor.SpanContext.TraceID() != consumidor.SpanContext.TraceID() {
		t.Fatal("produtor e consumidor ficaram em traces diferentes")
	}
	if consumidor.Parent.SpanID() != produtor.SpanContext.SpanID() {
		t.Fatal("o consumidor deveria ser filho do span do produtor")
	}
}

func TestInject_SemTraceAtivoDevolveNil(t *testing.T) {
	exportadorDeTeste(t)
	if got := Inject(context.Background()); got != nil {
		t.Fatalf("Inject sem span ativo = %v, quer nil (o campo some do JSON)", got)
	}
}

func TestExtract_CarrierVazioNaoMudaOContexto(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "x")
	if got := Extract(ctx, nil); got != ctx {
		t.Fatal("Extract(nil) deveria devolver o mesmo contexto")
	}
}

func TestFail_RegistraErroNoSpan(t *testing.T) {
	exp := exportadorDeTeste(t)
	_, span := Start(context.Background(), "op")
	Fail(span, nil) // nil não marca nada
	Fail(span, context.DeadlineExceeded)
	span.End()

	s := exp.GetSpans()[0]
	if s.Status.Code != codes.Error || len(s.Events) != 1 {
		t.Fatalf("status=%v eventos=%d, quer Error com 1 evento de erro", s.Status.Code, len(s.Events))
	}
}

func TestInit_SemEndpointFicaDesligadoMasMantemOPropagador(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	if Enabled() {
		t.Fatal("Enabled() deveria ser false sem endpoint")
	}
	shutdown, err := Init(context.Background(), "teste")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown do modo desligado falhou: %v", err)
	}
	// o propagador continua instalado: o serviço repassa o traceparent que recebe
	if len(otel.GetTextMapPropagator().Fields()) == 0 {
		t.Fatal("propagador não foi instalado")
	}
}

func TestEnabled_ReconheceOsDoisNomesDaVariavel(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://jaeger:4318/v1/traces")
	if !Enabled() {
		t.Fatal("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT deveria ligar o tracing")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://jaeger:4318")
	if !Enabled() {
		t.Fatal("OTEL_EXPORTER_OTLP_ENDPOINT deveria ligar o tracing")
	}
}
