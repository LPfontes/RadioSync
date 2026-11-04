# Plano de testes (versão preliminar — Entrega 2)

Cada teste existe para **validar uma decisão arquitetural**. Os resultados
serão anexados na Entrega 3 em `docs/resultados-testes.md`.

## 1. Testes unitários (já no CI)

Rodam em todo push com `go test -race ./...`.

| Pacote | O que verifica | Arquivo |
|---|---|---|
| `auth` | JWT do DJ: válido, outra estação, outro segredo, sem claim `dj`, `alg=none` | `internal/auth/jwt_test.go` |
| `model` | `calcPosition` (núcleo da sincronização) e extração de URL do YouTube | `internal/model/station_test.go` |
| `handler` | Código da estação (alfabeto, tamanho, colisões) e ID do YouTube | `internal/handler/station_test.go` |
| `media` | Formatos de áudio aceitos/recusados | `internal/media/convert_test.go` |
| `ws` | Fan-out do Hub para todos os clientes e detecção de DJ | `internal/ws/hub_test.go` |

> O teste de `ExtractYouTubeURL` encontrou um bug: uma sugestão com texto e
> link ("toca essa youtube.com/watch?v=x por favor") virava uma URL inválida.
> Corrigido em `model/station.go`.

**Meta Entrega 3:** cobertura ≥ 50% nos pacotes `auth`, `model`, `ws`.

## 2. Testes de integração

| ID | Cenário | Valida | Critério de sucesso |
|---|---|---|---|
| INT-01 | DJ conectado na réplica A envia `PLAY`; ouvinte na réplica B recebe | ADR-0001 | Ouvinte recebe em < 200 ms |
| INT-02 | Upload → job na fila → worker → `TRACK_READY` | ADR-0002 | Faixa aparece no repositório; resposta HTTP `202` em < 500 ms |
| INT-03 | Rotas `/api/v1/admin/*` sem token | ADR-0005 | Respondem `401` |
| INT-04 | Estação criada via réplica A é lida via réplica B | ADR-0006 | Mesmos dados |
| INT-05 | Rotas migradas pelo gateway respondem igual ao monolito | ADR-0003 | Mesmo status e corpo |

Ferramenta: testes Go com `httptest` + Docker Compose (ou Testcontainers).

## 3. Testes de carga (k6)

| ID | Cenário | Métrica | Meta |
|---|---|---|---|
| LOAD-01 | Rampa de 0 → 500 ouvintes WebSocket numa estação | Latência de propagação do `PLAY` (p95) | < 200 ms |
| LOAD-02 | Mesmo cenário com 1, 2 e 4 réplicas do Sync Service | Conexões suportadas / uso de CPU | Escala ~linear |
| LOAD-03 | 20 uploads simultâneos enquanto 200 ouvintes escutam | p95 do WebSocket antes vs. depois da fila | Degradação < 20% com fila |
| LOAD-04 | Rajada de `POST /stations` acima do limite | Respostas `429` | Gateway bloqueia excedente |
| LOAD-05 | Desvio de sincronização entre 100 clientes simulados | Diferença máxima de posição (p95) | < 500 ms |

Os scripts ficarão em `tests/load/` e as métricas serão coletadas no Grafana.

## 4. Testes de resiliência

| ID | Falha injetada | Comportamento esperado |
|---|---|---|
| RES-01 | `docker stop` de uma réplica do Sync Service com ouvintes conectados | Clientes reconectam em outra réplica e recebem o estado atual em < 5 s |
| RES-02 | Worker morto no meio de uma conversão | Mensagem volta à fila (sem ack) e é processada por outro worker |
| RES-03 | yt-dlp falhando sempre (URL inválida / rede bloqueada) | 3 retries → DLQ; após 5 falhas o circuit breaker abre |
| RES-04 | Redis reiniciado | Sync Service reconecta; eventos voltam a fluir; estado recuperado (AOF) |
| RES-05 | Postgres indisponível | Ouvintes continuam escutando (estado no Redis); criação de estação falha com erro claro |

## 5. Ambiente

- Execução local com `docker compose up -d --build`.
- Réplicas: `docker compose up -d --scale sync=3` (quando o Sync Service existir).
- Coleta: Prometheus (`:9090`) + Grafana (`:3000`) e relatórios do k6.
