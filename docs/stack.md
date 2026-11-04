# Stack tecnológico e justificativas

## Já em uso

| Camada | Tecnologia | Justificativa |
|---|---|---|
| Backend | **Go 1.26** | Goroutines baratas para milhares de conexões WebSocket simultâneas; binário único e imagem Docker pequena. |
| Roteador HTTP | **chi v5** | Compatível com `net/http`, middlewares simples, roteamento por grupos (`/api/v1`). |
| WebSocket | **gorilla/websocket** | Biblioteca madura, com controle de ping/pong e deadlines. |
| Autenticação | **golang-jwt v5** (HS256) | Token do DJ assinado por estação, sem precisar de sessão no servidor (stateless). |
| Áudio | **FFmpeg → Opus 96 kbps VBR** | Opus tem qualidade alta em bitrate baixo, reduzindo banda por ouvinte (ADR-0004). |
| Importação | **yt-dlp** + bgutil PO Token Provider | Única forma confiável de obter áudio do YouTube hoje. |
| Frontend | **Vue 3 + Vite + Pinia + Vue Router** | Reatividade adequada ao estado vindo do WebSocket; build rápido. |
| Container | **Docker multi-stage** | Uma imagem com frontend compilado + binário Go + FFmpeg. |

## Adicionados para a arquitetura distribuída (Entrega 2 → 3)

| Componente | Tecnologia | Justificativa | ADR |
|---|---|---|---|
| API Gateway / LB | **Traefik v3** | Descobre réplicas automaticamente via Docker; rate limit e métricas Prometheus nativos. | 0005 |
| Estado + Pub/Sub | **Redis 7** | Latência sub-ms para o fan-out dos eventos do player; mesmo componente guarda o estado quente. | 0001 |
| Fila de mensagens | **RabbitMQ 3.13** | Ack manual, DLQ e retry nativos para jobs longos de transcodificação. | 0002 |
| Banco relacional | **PostgreSQL 16** | Dados duráveis e relacionais (estações ↔ faixas); substitui o `stations.json`. | 0006 |
| Objetos | **MinIO** (API S3) | Arquivos de áudio acessíveis por todas as réplicas e workers; roda local no Compose. | 0006 |
| Observabilidade | **Prometheus + Grafana** | Métricas de latência (p95), conexões ativas e tamanho de fila; diferencial no critério de avaliação. | — |
| CI | **GitHub Actions** | Integrado ao repositório; roda vet, testes com `-race`, build do frontend e da imagem. | — |
| Testes de carga | **k6** (com módulo WebSocket) | Scripts em JS, suporte nativo a WebSocket e métricas p95/p99. | — |

## Bibliotecas Go previstas

| Uso | Biblioteca |
|---|---|
| Redis | `github.com/redis/go-redis/v9` |
| RabbitMQ | `github.com/rabbitmq/amqp091-go` |
| Postgres | `github.com/jackc/pgx/v5` |
| MinIO / S3 | `github.com/minio/minio-go/v7` |
| Circuit breaker | `github.com/sony/gobreaker/v2` |
| Métricas | `github.com/prometheus/client_golang` |
