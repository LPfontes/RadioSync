# ADR-0009: Tracing distribuído com OpenTelemetry e Jaeger

- **Status:** Implementado (Entrega 3, item opcional da frente de Observabilidade)
- **Data:** 2026-11-24
- **Responsável:** Membro 5 — João Vitor Sampaio (Testes + Observabilidade)
- **Áreas:** Observabilidade, Diagnóstico

## Contexto

As métricas (Prometheus/Grafana) dizem **que** algo está lento ou falhando, mas
não **onde**. Um upload passa por quatro processos: o gateway (Traefik), a Media
API, a fila (RabbitMQ) e o worker. Um comando do DJ passa pelo Sync Service e
depois pelo Redis Pub/Sub até as outras réplicas. Quando o p95 sobe, as métricas
por serviço não mostram em qual salto o tempo foi gasto, e os logs de cada
processo não se juntam sozinhos.

## Decisão

1. **OpenTelemetry** (SDK em Go, exportador **OTLP/HTTP**) como padrão de
   instrumentação, e **Jaeger v2** como backend e interface de consulta.
2. Pacote `internal/tracing`: `Init`/`Setup`, middleware HTTP, e
   `Inject`/`Extract` para atravessar mensagens.
3. **Propagação W3C `traceparent`:**
   - gateway → serviços pelo cabeçalho HTTP (o Traefik abre o trace);
   - Media API → worker pelo campo `trace` dentro do `Job` da fila;
   - réplica do Sync → outras réplicas pelo campo `trace` do evento do Redis.
4. **Desligado por padrão.** Sem `OTEL_EXPORTER_OTLP_ENDPOINT` o serviço não
   exporta nada e o custo é desprezível. No Compose a variável aponta para o
   Jaeger e pode ser esvaziada para desligar (`OTEL_EXPORTER_OTLP_ENDPOINT=`).
5. O **nome do span** é `MÉTODO /rota/{padrão}`, nunca a URL real (mesma regra
   das métricas, para não criar um nome por estação).
6. Ficam **fora** do tracing: `/metrics`, `/healthz` e o upgrade de WebSocket.

## O que um trace mostra

Upload (um trace só, 7 spans, 3 serviços):

```
POST                                   [gateway]
  ReverseProxy                         [gateway]
    POST /api/v1/stations/{stationId}/upload   [radiosync-media]
      queue.publish media.transcode            [radiosync-media]   (producer)
        media.job upload                       [radiosync-worker]  (consumer)
          media.process upload                 [radiosync-worker]
          redis.publish TRACK_READY            [radiosync-worker]
```

Comando do DJ com 2 réplicas do Sync:

```
ws.command PLAY            [réplica A]
  redis.publish STATE_UPDATE   [réplica A]
    redis.fanout STATE_UPDATE  [réplica B]
```

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Só logs correlacionados por ID | Dá para seguir um job, mas não mostra a árvore nem o tempo de cada salto; exigiria escrever o que o tracing já faz |
| Zipkin / Tempo | Jaeger v2 já fala OTLP nativamente, sobe com uma imagem e tem UI de consulta; o Tempo pede também um Grafana configurado para consultá-lo |
| `otelhttp` (contrib) | Nomeia o span pela URL; o nosso middleware usa o padrão da rota. Evita também mais uma dependência |
| Contexto no cabeçalho AMQP | Não sobrevive a retry e DLQ, que republicam o `Job`; no corpo ele segue junto e funciona igual com a fila em memória |
| Amostragem reduzida por padrão | Com o tráfego atual 100% é viável; a amostragem fica configurável por `OTEL_TRACES_SAMPLER` e `OTEL_TRACES_SAMPLER_ARG` |

## Consequências

**Positivas:** é possível ver, num trace, em qual salto um upload ou um comando
do DJ gastou tempo, e se o fan-out chegou à outra réplica. O retry de um job
continua no mesmo trace (o `Job` republicado leva o `trace` original).

**Negativas e riscos:**

- O Jaeger guarda os traces **em memória**: reiniciar o container apaga tudo.
  Para o escopo do projeto basta; em produção precisaria de armazenamento.
- Cada comando do DJ vira um trace; com muitos DJs o volume cresce. A amostragem
  existe justamente para isso, mas não está ligada.
- Entram dependências novas (`go.opentelemetry.io/*`, `google.golang.org/grpc`
  e relacionadas), todas passando em `govulncheck` e Trivy no momento da entrada.
- O trace termina onde o contexto não viaja: não há spans de Postgres, MinIO nem
  dos comandos do Redis além do Pub/Sub.

## Como usar

```bash
docker compose up -d --build
# gere tráfego (um upload, um PLAY do DJ) e abra:
open http://localhost:16686
```

No Jaeger, escolha o serviço `gateway` e procure por `POST` para ver o trace
inteiro de um upload, ou `radiosync-sync` e `ws.command PLAY` para o fan-out.

Variáveis: `OTEL_EXPORTER_OTLP_ENDPOINT` (vazio desliga), `OTEL_SERVICE_NAME`,
`OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`, `JAEGER_UI_BIND` (padrão
`127.0.0.1`).
