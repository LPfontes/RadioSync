// Package tracing configura o OpenTelemetry (traces) e oferece o que os serviços
// usam: inicialização, middleware HTTP e propagação do contexto pela fila e pelo
// Redis Pub/Sub.
//
// O tracing fica DESLIGADO por padrão: sem OTEL_EXPORTER_OTLP_ENDPOINT nada é
// exportado e o custo é praticamente zero (a API do OpenTelemetry vira no-op).
// No Compose a variável aponta para o Jaeger, e os traces aparecem em
// http://localhost:16686.
package tracing

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "radio-backend"

// Enabled informa se há um destino OTLP configurado.
func Enabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}

// Init configura o tracing do serviço e devolve a função que descarrega os
// spans pendentes (chame no encerramento). Segue as variáveis padrão do
// OpenTelemetry: OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_SERVICE_NAME,
// OTEL_TRACES_SAMPLER e OTEL_TRACES_SAMPLER_ARG.
func Init(ctx context.Context, service string) (shutdown func(context.Context) error, err error) {
	// O propagador é sempre instalado: mesmo com o tracing desligado aqui, o
	// serviço repassa o traceparent que recebeu.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	noop := func(context.Context) error { return nil }
	if !Enabled() {
		return noop, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, err
	}

	host, _ := os.Hostname()
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", service),
			attribute.String("service.instance.id", host),
		),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES têm a palavra final
	)
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Start abre um span filho do que estiver no ctx.
func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartProducer abre o span de quem publica uma mensagem (fila ou Pub/Sub).
func StartProducer(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attrs...))
}

// StartConsumer abre o span de quem processa uma mensagem, como filho do span
// que a publicou (carrier vem do Inject do produtor). Sem carrier, abre um trace novo.
func StartConsumer(ctx context.Context, name string, carrier map[string]string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx = Extract(ctx, carrier)
	return otel.Tracer(tracerName).Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attrs...))
}

// Inject serializa o contexto do trace num mapa que viaja dentro da mensagem
// (campo do Job na fila, campo do evento no Redis). Devolve nil se não houver
// trace ativo, para o campo sumir do JSON (omitempty).
func Inject(ctx context.Context) map[string]string {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return nil
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if len(carrier) == 0 {
		return nil
	}
	return carrier
}

// Extract é o inverso de Inject.
func Extract(ctx context.Context, carrier map[string]string) context.Context {
	if len(carrier) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
}

// Fail marca o span como erro, com o motivo.
func Fail(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// HTTPMiddleware cria um span por requisição HTTP, filho do traceparent que o
// gateway enviou.
//
// O nome do span é "MÉTODO /rota/{padrão}", nunca a URL real, pelo mesmo motivo
// das métricas: com a URL real cada estação seria um nome diferente. Ficam de
// fora /metrics, /healthz e o upgrade de WebSocket (a conexão dura horas e viraria
// um span gigante; os comandos do DJ têm span próprio no Sync Service).
//
// Deve ficar ANTES do Recoverer, como o middleware de métricas, para que um
// panic recuperado como 500 marque o span como erro.
func HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Um span não pode ser descartado depois de aberto, então o que fica de
			// fora é decidido aqui, antes: raspagem do Prometheus, healthcheck e
			// upgrade de WebSocket.
			if r.URL.Path == "/metrics" || r.URL.Path == "/healthz" ||
				strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				next.ServeHTTP(w, r)
				return
			}

			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := otel.Tracer(tracerName).Start(ctx, r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("radiosync.service", service),
				))

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))

			route := routePattern(r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.String("url.path_template", route),
				attribute.Int("http.response.status_code", status),
			)
			if status >= 500 {
				span.SetStatus(codes.Error, strconv.Itoa(status))
			}
			span.End()
		})
	}
}

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return "unmatched"
}

// Setup liga o tracing do serviço e devolve a função de encerramento, para uso
// direto no main: `defer tracing.Setup("radiosync-sync")()`. Falha de
// configuração não derruba o serviço: observabilidade não pode tirar o app do ar.
func Setup(service string) func() {
	shutdown, err := Init(context.Background(), service)
	if err != nil {
		log.Printf("AVISO: tracing desligado: %v", err)
		return func() {}
	}
	if Enabled() {
		log.Printf("tracing ligado (%s) -> %s", service, endpointDescrito())
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			log.Printf("AVISO: descarregar spans: %v", err)
		}
	}
}

func endpointDescrito() string {
	if v := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); v != "" {
		return v
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
}
