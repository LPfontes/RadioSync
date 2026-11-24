# 📻 RadioSync — Rádio Web Sincronizada em Tempo Real

**RadioSync** é uma plataforma distribuída para transmissão e escuta sincronizada de rádio online em tempo real. Permite que um **DJ** crie uma estação, faça upload de músicas ou importe do YouTube (convertidas para `.opus`), gerencie a fila de reprodução e o mixer de efeitos sonoros, e sincronize os controles (`PLAY`, `PAUSE`, `SEEK`, troca de faixa) com centenas de **Ouvintes** conectados via WebSockets.

> **Evolução Arquitetural (Do Monolito para Microsserviços via Strangler Fig):**  
> O projeto nasceu como um **monolito Go em processo único** (`backend/cmd/server` — preservado para comparação na branch **`monolito`**, commit `df075d2`), onde a API REST, o Hub WebSocket, a transcodificação via FFmpeg/yt-dlp, a persistência em arquivos JSON locais (`stations.json`, `track_names.json`, `yt_cache.json`) e a entrega dos arquivos `.opus` residiam na mesma instância. Para viabilizar escalabilidade horizontal, tolerância a falhas e isolamento de carga sem interromper o funcionamento do sistema, a arquitetura foi evoluída incrementalmente aplicando o padrão **Strangler Fig** ([ADR-0003](docs/adr/0003-migracao-strangler-fig.md)): um API Gateway (**Traefik**) foi posicionado na borda e as responsabilidades críticas foram extraídas para serviços independentes e *stateless* (**Sync Service** com Redis Pub/Sub, **Station Service** com PostgreSQL + MinIO e **Media Service + Worker** com fila RabbitMQ), mantendo o servidor base apenas para as rotas ainda não migradas e entrega da SPA.

---

## 🚀 Arquitetura e Tecnologias Utilizadas

### **Backend e Microsserviços (Go 1.26)**
- **API Gateway / Borda**: [Traefik v3.6](https://traefik.io/) — roteamento Strangler Fig, load balancing, ForwardAuth administrativo e rate limiting por rota ([ADR-0003](docs/adr/0003-migracao-strangler-fig.md), [ADR-0005](docs/adr/0005-api-gateway-seguranca.md)).
- **Sync Service (`backend/cmd/sync`)**: gerenciamento escalável de conexões WebSocket, fan-out em tempo real via **Redis Pub/Sub**, lock distribuído de DJ com heartbeat e sincronização de relógio estilo NTP ([ADR-0001](docs/adr/0001-redis-pubsub-fanout-websocket.md), [ADR-0004](docs/adr/0004-sincronizacao-client-side-timestamp.md)).
- **Station Service (`backend/cmd/station`)**: serviço stateless de estações, playlist, biblioteca global, `claim-dj`, emissão/renovação deslizante de JWT de DJ e entrega de áudio `.opus` a partir do **MinIO** ([ADR-0003](docs/adr/0003-migracao-strangler-fig.md), [ADR-0006](docs/adr/0006-persistencia-postgres-minio.md)).
- **Importador de Dados (`backend/cmd/importer`)**: migração idempotente dos arquivos legados (`stations.json`, `track_names.json`, `tracks.json`, `yt_cache.json` e `.opus`) para PostgreSQL e MinIO.
- **Media API (`backend/cmd/media`)**: recebe upload e link do YouTube, guarda o original no MinIO, publica o job no RabbitMQ e responde `202` com `jobId` ([ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md)).
- **Media Worker (`backend/cmd/worker`)**: converte com **FFmpeg** (Opus 96 kbps VBR) e baixa com **yt-dlp** (+ PO Token Provider `bgutil`), com ack manual, idempotência, retry com backoff, DLQ e circuit breaker. É a única imagem com FFmpeg e yt-dlp ([ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md)).
- **Servidor Base (`backend/cmd/server`)**: rotas administrativas e entrega da SPA.

### **Persistência Poliglota e Mensageria**
- **PostgreSQL 16**: persistência relacional durável de estações, catálogo global de faixas, itens ordenados da playlist, sugestões de ouvintes e cache do YouTube, com migrations SQL versionadas ([ADR-0006](docs/adr/0006-persistencia-postgres-minio.md)).
- **MinIO (S3-compatível)**: armazenamento distribuído de objetos de áudio no bucket `musicas` via interface `ObjectStore` (`Put`, `Get`, `PresignedGetURL`), eliminando dependência de disco local ([ADR-0006](docs/adr/0006-persistencia-postgres-minio.md)).
- **Redis 7 (AOF)**: armazenamento do estado quente de reprodução (`station:{id}:state`), lock global de DJ (`station:{id}:dj`) e barramento Pub/Sub (`station:{id}:events`) ([ADR-0001](docs/adr/0001-redis-pubsub-fanout-websocket.md)).
- **RabbitMQ 3.13**: fila de jobs de transcodificação (`media.transcode`) e Dead-Letter Queue (`media.transcode.dlq`) ([ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md)).

### **Frontend e Observabilidade**
- **Frontend**: [Vue.js 3](https://vuejs.org/) (Composition API), [Vite](https://vitejs.dev/), [Pinia](https://pinia.vuejs.org/), [Vue Router 4](https://router.vuejs.org/) e Design System em Vanilla CSS (tema escuro estilo YouTube Music).
- **Observabilidade**: [Prometheus](https://prometheus.io/) (`/metrics` nos serviços e gateway) + [Grafana](https://grafana.com/) com datasource provisionado automaticamente.

---

## 🛠️ Como Executar o Projeto

### 1. Ambiente Completo com Docker Compose (Recomendado)

Sobe todos os serviços da arquitetura distribuída (Traefik Gateway, Sync Service, Station Service, Media API, Media Worker, Servidor Base, Redis, RabbitMQ, PostgreSQL, MinIO, Prometheus e Grafana):

```bash
cp .env.example .env        # configure JWT_SECRET (mínimo 32 caracteres) e ADMIN_PASSWORD
docker compose up -d --build
```

| Serviço | Endereço / Comando |
|---|---|
| **Aplicação (via API Gateway Traefik)** | http://localhost |
| **Station Service (REST + Áudio MinIO via Gateway)** | http://localhost/api/v1/stations e http://localhost/api/v1/library |
| **Sync Service (WebSockets via Gateway)** | `ws://localhost/ws/stations/{id}` |
| **Escalar os serviços** | `docker compose up -d --scale sync=3 --scale station=2 --scale media-worker=3` |
| **Servidor Base (acesso direto)** | http://localhost:8080 |
| **Traefik Dashboard** (protegido por senha; gere com `./scripts/traefik-dashboard-password.sh`) | http://localhost:8081/dashboard/ |
| **RabbitMQ Management** | http://localhost:15672 |
| **MinIO Console** | http://localhost:9001 |
| **Prometheus** | http://localhost:9090 |
| **Grafana** | http://localhost:3000 |
| **Jaeger** (traces) | http://localhost:16686 |

---

### 2. Persistência Poliglota, Station Service e Importador Único

O **Station Service** (`backend/cmd/station`, [Dockerfile.station](Dockerfile.station)) foi extraído segundo o padrão **Strangler Fig** ([ADR-0003](docs/adr/0003-migracao-strangler-fig.md)) e implementa a arquitetura de **Persistência Poliglota** ([ADR-0006](docs/adr/0006-persistencia-postgres-minio.md)):

- **Migrations Versionadas no Postgres:** No boot, `storage.RunMigrations` aplica automaticamente os scripts SQL embutidos em `backend/internal/storage/migrations/` (`0001_initial_schema.up.sql`), criando as tabelas `tracks`, `stations`, `station_tracks`, `playlist_items`, `suggestions`, `youtube_cache` e `schema_migrations`.
- **Importador Único e Idempotente:** Na inicialização (ou manualmente via `go run ./cmd/importer`), o pacote `internal/importer` lê `stations.json`, `track_names.json`, `tracks.json` e `yt_cache.json`, faz `UPSERT` no PostgreSQL e envia todos os arquivos `.opus` locais para o bucket `musicas` no MinIO. O antigo `persist.go` foi removido.
- **Áudio Servido do MinIO sem Disco Local:** As requisições a `/musicas/{filename}` são atendidas a partir do `ObjectStore` no MinIO (via streaming HTTP com suporte a `Range` ou redirecionamento `302` / JSON de URL pré-assinada com `?presign=1`).

Para escalar horizontalmente o Station Service e o Sync Service:
```bash
docker compose up -d --scale station=2 --scale sync=3
```

---

### 3. Media Service + Worker (fila assíncrona)

Upload e YouTube são assíncronos ([ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md)): a Media API responde `202` na hora, o worker converte em outro container e a faixa chega ao painel por `TRACK_READY`.

```bash
# Enviar uma música e acompanhar (TOKEN é o djToken da estação)
curl -i -H "Authorization: Bearer $TOKEN" -F file=@musica.mp3 http://localhost/api/v1/stations/$ID/upload
curl -H "Authorization: Bearer $TOKEN" http://localhost/api/v1/stations/$ID/jobs

# Filas (inclusive a DLQ) e circuit breaker
docker compose exec rabbitmq rabbitmqctl list_queues name messages
docker compose exec media-worker curl -s localhost:8085/metrics | grep radiosync_media

# RES-02: matar um worker no meio de uma conversão
docker compose up -d --scale media-worker=2
docker kill -s KILL <container do worker>
```

Um job que esgota as tentativas fica em `media.transcode.dlq`, com o motivo no cabeçalho `x-error`, e pode ser visto em http://localhost:15672.

---

### 4. Executando Localmente (Desenvolvimento)

#### **Pré-requisitos**
- **Go** >= 1.26
- **Node.js** >= 20 e **npm**
- **PostgreSQL**, **Redis**, **RabbitMQ** e **MinIO** (podem ser subidos via `docker compose up -d postgres redis rabbitmq minio`)
- **FFmpeg**, **FFprobe** e **yt-dlp** no `PATH` (só para rodar o worker fora do Docker)

```bash
# Subir dependências de dados no Docker
docker compose up -d postgres redis rabbitmq minio

# Station Service (:8083)
cd backend
go run ./cmd/station/main.go

# Sync Service (:8082)
cd backend
go run ./cmd/sync/main.go

# Media API (:8084) e Media Worker (:8085)
cd backend
go run ./cmd/media/main.go
go run ./cmd/worker/main.go

# Servidor Base (:8080)
cd backend
go run ./cmd/server/main.go

# Frontend (:5173). As chamadas vão para o gateway em http://localhost; para
# outro endereço, defina VITE_PROXY_TARGET
cd frontend
npm install
npm run dev
```

---

### 5. Deploy Automatizado com Rollback

O deploy utiliza imagens imutáveis (tag = SHA do commit), verifica o `/healthz` com a versão nova e executa rollback automático caso a validação falhe ([ADR-0007](docs/adr/0007-estrategia-de-deployment.md)):

```bash
./scripts/deploy.sh           # constrói <imagem>:<sha> e atualiza a versão
./scripts/deploy.sh <sha>     # faz rollback/deploy para um SHA específico
curl -s localhost:8080/healthz
```

---

### 6. Testes Automatizados e Pipeline CI / DevSecOps

```bash
cd backend
go vet ./...
go test -v -race ./...
```

- **Testes Unitários e de Contrato:** pacotes `auth`, `breaker`, `jobs`, `mediaservice`, `model`, `handler`, `media`, `queue`, `security`, `storage`, `importer`, `ws`.
- **Testes de Integração e Resiliência:**
  - `INT-01`, `RES-01`, `RES-04`: `backend/internal/syncservice/integration_test.go` (fan-out Pub/Sub < 200 ms, queda de réplica < 5 s, reconexão ao Redis).
  - `INT-04`, `INT-05`, `RES-05`: `backend/internal/stationservice/integration_test.go` (consistência de estação entre Réplica A e Réplica B, paridade Strangler Fig com o monolito e degradação graciosa com Postgres fora do ar).
  - `INT-02`, `RES-02`, `RES-03`: `backend/internal/mediaworker` (upload → fila → worker → `TRACK_READY`, worker morto no meio da conversão, retry + DLQ + circuit breaker).
  - Com `TEST_DATABASE_URL`, `TEST_REDIS_URL` e `TEST_AMQP_URL`, os testes usam Postgres, Redis e RabbitMQ reais, como no CI.
- **CI (`.github/workflows/ci.yml`):** executa `go vet`, `go test -race` (com Postgres, Redis e RabbitMQ), build do frontend, validação do `docker-compose.yml` e build Docker.
- **DevSecOps (`.github/workflows/security.yml`, [ADR-0008](docs/adr/0008-devsecops-pipeline.md)):** executa `govulncheck`, `npm audit`, `gitleaks`, `gosec`, `Trivy` (imagem e configuração) e gera SBOM SPDX com `Syft`.

---

## 🏛️ Índice Completo da Documentação de Arquitetura

| Documento | Conteúdo |
|---|---|
| [C4 — Nível 1: Contexto](docs/c4/01-contexto.md) | Atores (DJ, Ouvinte, Admin) e sistemas externos (YouTube) |
| [C4 — Nível 2: Containers](docs/c4/02-containers.md) | Arquitetura de containers (Gateway, Sync, Station, Media/Worker, Redis, Postgres, MinIO, RabbitMQ) |
| [ADR-0000 — Template](docs/adr/0000-template.md) | Modelo padrão para registro de decisões arquiteturais |
| [ADR-0001 — Redis Pub/Sub Fan-out](docs/adr/0001-redis-pubsub-fanout-websocket.md) | Escalabilidade horizontal de WebSockets e estado quente no Redis |
| [ADR-0002 — Transcodificação Assíncrona](docs/adr/0002-transcodificacao-assincrona-fila.md) | Fila RabbitMQ, Worker idempotente, Retry com backoff, DLQ e Circuit Breaker |
| [ADR-0003 — Migração Strangler Fig](docs/adr/0003-migracao-strangler-fig.md) | Estratégia de extração incremental e tabela de rotas migradas no gateway |
| [ADR-0004 — Sincronização Client-Side](docs/adr/0004-sincronizacao-client-side-timestamp.md) | Sincronização por timestamp e compensação de offset de relógio estilo NTP |
| [ADR-0005 — API Gateway e Segurança](docs/adr/0005-api-gateway-seguranca.md) | Traefik na borda, JWT HS256, ForwardAuth admin, CORS/Origin e Rate Limiting |
| [ADR-0006 — Persistência Poliglota](docs/adr/0006-persistencia-postgres-minio.md) | PostgreSQL 16 (metadados), MinIO (áudio `.opus`) e Redis (estado quente) |
| [ADR-0007 — Estratégia de Deployment](docs/adr/0007-estrategia-de-deployment.md) | Imagens com tag imutável por SHA, `/healthz` e rollback automatizado |
| [ADR-0008 — Pipeline DevSecOps](docs/adr/0008-devsecops-pipeline.md) | Verificações shift-left com govulncheck, npm audit, gitleaks, gosec, Trivy e SBOM |
| [Stack Tecnológico](docs/stack.md) | Tecnologias adotadas e justificativas arquiteturais |
| [Padrões Arquiteturais](docs/padroes.md) | Padrões da disciplina aplicados ao RadioSync |
| [Plano de Testes](docs/plano-de-testes.md) | Especificação dos testes unitários, integração (`INT-*`), carga (`LOAD-*`) e resiliência (`RES-*`) |
| [Divisão de Responsabilidades](docs/divisao-responsabilidades.md) | Frentes e entregáveis de cada um dos 5 integrantes |

---

## ⚙️ Variáveis de Ambiente

| Variável | Valor Padrão | Descrição |
| :--- | :--- | :--- |
| `PORT` | `8080` / `8082` / `8083` / `8084` / `8085` | Porta HTTP do serviço (`server`, `sync`, `station`, `media`, `worker`). |
| `JWT_SECRET` | _(obrigatório em prod)_ | Segredo HS256 dos tokens JWT, com no mínimo 32 caracteres (`openssl rand -hex 32`). |
| `DJ_TOKEN_TTL` | `24h` | Validade do token de DJ (com renovação deslizante ao abrir a sala). |
| `ADMIN_PASSWORD` | _(vazio = 503)_ | Senha do painel `/admin`. Quando vazia, desabilita as rotas administrativas (`503`). |
| `ADMIN_TOKEN_TTL` | `8h` | Validade do JWT administrativo emitido no login. |
| `ALLOWED_ORIGINS` | _(vazio)_ | Origens extras permitidas no CORS e no handshake WebSocket (separadas por vírgula). |
| `DATABASE_URL` | `postgres://radiosync:...@postgres:5432/radiosync?sslmode=disable` | String de conexão do PostgreSQL 16 usada pelo Station Service e pelo Importador. |
| `S3_ENDPOINT` | `http://minio:9000` | Endpoint do servidor MinIO / S3. |
| `S3_BUCKET` | `musicas` | Nome do bucket no MinIO onde os arquivos `.opus` são armazenados. |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `radiosync` / `radiosync123` | Credenciais de acesso ao MinIO. |
| `AUDIO_DELIVERY_MODE` | `proxy` | Modo de entrega em `/musicas/{filename}`: `proxy` (stream direto do MinIO) ou `presigned` (redirect `302` para URL pré-assinada). |
| `REDIS_URL` | `redis://redis:6379/0` | Endereço do Redis para estado quente, lock de DJ e Pub/Sub. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://jaeger:4318` (no Compose) | Destino dos traces (OpenTelemetry). **Vazio desliga o tracing.** Ver ADR-0009. |
| `OTEL_TRACES_SAMPLER_ARG` | _(1.0)_ | Fração de traces amostrados, com `OTEL_TRACES_SAMPLER=parentbased_traceidratio`. |
| `AMQP_URL` | `amqp://radiosync:...@rabbitmq:5672/` | Endereço do RabbitMQ para fila de transcodificação de mídia. |
| `MEDIA_RETRY_DELAYS` | `5s,30s,2m` | Atrasos entre as novas tentativas de um job; depois da última, DLQ. Use o mesmo valor na Media API e no worker. |
| `MEDIA_JOB_TIMEOUT` | `10m` | Tempo máximo de uma conversão ou download. |
| `WORKER_CONCURRENCY` / `WORKER_CPUS` / `WORKER_MEMORY` | `2` / `1.5` / `1g` | Jobs simultâneos e limites de cada réplica do worker. |
| `YT_BREAKER_FAILURES` / `YT_BREAKER_COOLDOWN` | `5` / `60s` | Falhas seguidas que abrem o circuit breaker do YouTube e tempo aberto. |
| `DATA_DIR` / `MUSIC_DIR` | `/app/data` / `/app/musicas` | Diretórios lidos pelo importador na migração dos arquivos JSON e `.opus` legados. |

---

## 📂 Estrutura de Diretórios

```text
RadioSync/
├── .github/workflows/
│   ├── ci.yml                          # Pipeline CI (go vet, go test -race, build frontend e docker)
│   └── security.yml                    # Pipeline DevSecOps (govulncheck, npm audit, gitleaks, gosec, Trivy, SBOM)
├── backend/
│   ├── cmd/
│   │   ├── importer/main.go            # CLI do importador único (JSONs + .opus -> Postgres + MinIO)
│   │   ├── media/main.go               # Media API (upload e YouTube assíncronos)
│   │   ├── server/main.go              # Servidor base (Admin e arquivos estáticos da SPA)
│   │   ├── station/main.go             # Station Service extraído (Postgres + MinIO + Redis)
│   │   ├── sync/main.go                # Sync Service extraído (WebSockets + Redis Pub/Sub)
│   │   └── worker/main.go              # Media Worker (FFmpeg + yt-dlp)
│   ├── internal/
│   │   ├── auth/                       # Geração/validação JWT de DJ e Admin
│   │   ├── breaker/                    # Circuit breaker das chamadas ao YouTube
│   │   ├── handler/                    # Handlers REST, entrega de áudio do MinIO e middlewares
│   │   ├── importer/                   # Importador idempotente de stations.json, track_names.json e yt_cache.json
│   │   ├── jobs/                       # Andamento e locks dos jobs de mídia (Redis)
│   │   ├── media/                      # Wrappers FFmpeg, ffprobe e yt-dlp (com proteção contra argument injection)
│   │   ├── mediaservice/               # Media API (validação, MinIO, publicação do job)
│   │   ├── mediaworker/                # Media Worker e testes INT-02, RES-02 e RES-03
│   │   ├── metrics/                    # Coletores e endpoint Prometheus (/metrics)
│   │   ├── model/                      # Modelos de domínio (Station, Track, PlaybackState, SongSuggestion, Mixer)
│   │   ├── queue/                      # Fila media.transcode, retry e DLQ (RabbitMQ)
│   │   ├── security/                   # Validação de CORS e Origin de WebSocket
│   │   ├── stationservice/             # Implementação do Station Service e testes INT-04, INT-05 e RES-05
│   │   ├── storage/                    # Interfaces Repository/ObjectStore, Postgres, MinIO e migrations SQL
│   │   ├── syncservice/                # Engine distribuída do Sync Service e testes INT-01, RES-01 e RES-04
│   │   └── ws/                         # Cliente/Hub WebSocket e estimativa de offset de relógio NTP
│   ├── go.mod
│   └── go.sum
├── docs/
│   ├── adr/                            # Architecture Decision Records (0000 a 0008)
│   ├── c4/                             # Diagramas C4 Nível 1 e Nível 2 em Mermaid
│   ├── divisao-responsabilidades.md
│   ├── padroes.md
│   ├── plano-de-testes.md
│   └── stack.md
├── frontend/                           # SPA Vue 3 + Vite + Pinia
├── infra/                              # Configurações de Traefik, Prometheus e Grafana
├── scripts/                            # Scripts de deploy com rollback e geração de senha do Traefik
├── Dockerfile                          # Build multi-stage do servidor base + frontend
├── Dockerfile.media                    # Build multi-stage enxuto da Media API
├── Dockerfile.station                  # Build multi-stage enxuto do Station Service + Importer
├── Dockerfile.sync                     # Build multi-stage enxuto do Sync Service
├── Dockerfile.worker                   # Media Worker, única imagem com FFmpeg e yt-dlp
├── docker-compose.yml                  # Orquestração completa da arquitetura distribuída
└── README.md
```

---

## 🤖 Ferramentas de IA utilizadas

> Seção obrigatória pelo documento da disciplina — a omissão desclassifica a
> entrega. **Cada integrante deve revisar e completar a parte da sua frente.**

| Ferramenta | Onde atuou | Como foi orientada | Avaliação honesta |
|---|---|---|---|
| Claude (Anthropic) | Entrega 2: rascunho dos ADRs 0001–0006, diagramas C4 em Mermaid, plano de testes, `docker-compose.yml` com a infraestrutura, workflow de CI e testes unitários Go | Recebeu o documento do projeto final e o link do repositório; leu o código atual e gerou os artefatos a partir dele | Os testes encontraram um bug real em `ExtractYouTubeURL` (corrigido). A equipe revisou os ADRs e ajustou: _descrever aqui o que foi alterado ou descartado_ |
| Claude (Anthropic), via Claude Code — João Vitor Sampaio Costa (Membro 5) | **Entrega 2:** C4 nível 1, template de ADR, padrões arquiteturais, plano de testes e configuração inicial de Prometheus/Grafana. **Entrega 3, frente de Observabilidade e Testes (#19):** pacote `internal/metrics` (middleware HTTP com latência p95 por rota, erros e `/metrics` do monólito), dashboard do Grafana provisionado como código, scripts k6 LOAD-01 a LOAD-05, testes unitários de `model` e `ws`, harness de integração, scripts de resiliência com falha real (RES-01 a RES-05), ferramenta de stack isolada, tracing com OpenTelemetry e Jaeger (ADR-0009), correção do Security do CI (govulncheck e Trivy), C4 nível 1 atualizado e `docs/resultados-testes.md`. Também executou os testes contra o Compose completo, tirou as capturas do Grafana e fez os commits, PRs e merges. | Recebeu o texto da issue #19 e acesso ao repositório e ao Docker; escreveu os scripts, rodou os testes e leu os resultados. As decisões de escopo e de processo vieram do João (PRs para a `dev`, só mergear com CI verde, sem coautoria, não parar a stack dele). | **O que o Claude errou e como apareceu:** (1) o middleware de métricas contava o WebSocket como `200` e só ao fechar a conexão; o teste unitário passou porque simulava um comportamento que o `gorilla/websocket` não tem, e o erro só apareceu ao olhar uma captura do Grafana (corrigido no PR #40, com um teste que falha no código antigo); (2) o Prometheus misturava as réplicas do sync numa série só e não raspava o `station` (achado ao rodar o LOAD-02); (3) atualizar o `gorilla/websocket` puxou um `golang.org/x/net` vulnerável e quebrou o Trivy; (4) commitou por engano um resultado do k6 com um JWT de teste e o gitleaks barrou o PR; (5) um primeiro LOAD-01 foi invalidado porque o Mac entrou em repouso, e o resultado foi descartado e refeito; (6) numa ocasião fechou janelas do navegador que não eram do teste. **Limites do que foi entregue:** cada cenário rodou uma ou duas vezes numa máquina só, sem medir variância, e o LOAD-02 não demonstra escala (a carga não satura uma réplica). Os números e esses limites estão em `docs/resultados-testes.md`. |
| Gemini (Google DeepMind), via Antigravity — Nathan (Membro 1) | Entrega 3, frente do Sync Service (WebSocket e sincronização distribuída): extração do binário `backend/cmd/sync` e serviço `sync` escalável no compose (`--scale sync=N`), fan-out entre réplicas via Redis Pub/Sub (`station:{id}:events`, ADR-0001), estado quente do `PlaybackState` no Redis (`station:{id}:state`), lock global de 1 DJ por estação com TTL e heartbeat (`station:{id}:dj`), algoritmo client-side estilo NTP para estimativa e compensação de offset de relógio (ADR-0004) no backend (`ws.EstimateClockOffset`) e frontend (`usePlayer.js` / `useWebSocket.js`), reconexão transparente, retenção de segurança de subprotocolo e CORS/Origin, pacote de observabilidade com Prometheus (`internal/metrics`), testes unitários de pubsub e offset, e testes automatizados de integração INT-01 (< 200 ms), RES-01 (< 5 s) e RES-04 (reinício do Redis) | Guiada com os requisitos da Entrega 3, especificações dos ADRs 0001, 0003 e 0004 e contratos de dependência com as demais frentes (Membro 3 no gateway, Membro 5 nas métricas e Membro 4 no storage). Foi solicitado implementar a extração completa do serviço com testes automatizados executáveis com race detector | **Funcionou:** projetou e implementou com precisão a arquitetura de fan-out com supressão de auto-mensagens (`senderId`), testes automatizados INT-01 (latência medida de 1,1 ms) e RES-01 (reconexão medida em 1,7 ms), e o algoritmo NTP eliminou completamente distorções de playback causadas por relógios locais desajustados. **Precisou ser corrigido:** o design inicial colocou a engine dentro de `internal/ws`, o que gerou ciclo de importação com `internal/model` e `internal/handler` (resolvido extraindo a camada de orquestração para `internal/syncservice`), e a primeira versão do mock de broker para testes unitários precisou de ajuste para rastrear o ID de cada assinante |
| Claude (Anthropic), via Claude Code — Gabriel Ribeiro (Membro 3) | Entrega 3, frente de Segurança, DevSecOps e Deployment (#17): auth do admin com JWT e ForwardAuth, CORS e `Origin` do WebSocket, validação do `JWT_SECRET` e expiração do token de DJ, token fora da URL do WebSocket, rate limit por rota, senha do dashboard do Traefik, correção da injeção de argumentos no yt-dlp, workflow `security.yml`, ADR-0007, ADR-0008, `scripts/deploy.sh` e `scripts/traefik-dashboard-password.sh`, com testes Go de cada item. Também ajudou a dividir a Entrega 3 nas issues #15 a #19 | Em sessões no próprio repositório, com o documento do projeto e a issue como contexto; pedi um item por vez, cada um em PR para `dev`. A IA subia a stack local com Docker Compose e testava cada mudança (curl, navegador, clientes WebSocket) antes de abrir o PR | **Funcionou:** os scanners que ela configurou acharam uma falha crítica real (a URL do YouTube virava opção do yt-dlp) e 21 vulnerabilidades da biblioteca padrão do Go. **Precisou ser corrigido:** várias suposições dela estavam erradas e só caíram nos testes. Ela achou que o restart perdia até 30 s de estado (não perdia); mediu 0,4 s de deploy quando o gateway ficava ~10 s sem a versão nova; afirmou que o Traefik relia a senha do dashboard sozinho (não relê); o script de senha deixava a senha aparecer na tela; e o govulncheck passou na máquina local e falhou no CI. **Descartado:** blue-green e canary por enquanto, por causa do estado em memória (ADR-0007). Os PRs desta frente foram mergeados sem a aprovação de outro membro, por decisão minha, para cumprir o prazo |
| Gemini (Google DeepMind), via Antigravity — Luiz Paulo (Membro 4) | Entrega 3, frente de Dados e Station Service (#18): definição dos contratos `storage.Repository` e `storage.ObjectStore` (`backend/internal/storage`), schema PostgreSQL com migrations versionadas via `embed.FS` (`0001_initial_schema.up.sql` / `.down.sql`), implementação do `PostgresRepository` e do `MinIOObjectStore` (bucket `musicas` com `Put`, `Get` e `PresignedGetURL` via AWS Signature V4), importador único e idempotente (`backend/internal/importer` e `backend/cmd/importer`) para `stations.json`, `track_names.json`, `yt_cache.json` e arquivos `.opus`, entrega de áudio `/musicas/*` diretamente do MinIO sem disco local, extração do microsserviço `backend/cmd/station` (`Dockerfile.station`) com roteamento Strangler Fig no Traefik (`docker-compose.yml`), remoção do `persist.go`, testes automatizados `INT-04`, `INT-05` e `RES-05`, atualização dos ADRs 0003 e 0006 e consolidação deste README | Orientada a partir do escopo da Frente 4 (`docs/divisao-responsabilidades.md`, `docs/plano-de-testes.md`, ADR-0003 e ADR-0006), dividindo o trabalho em 5 etapas/commits incrementais e validando cada etapa com `go vet` e `go test -race ./...` | **Funcionou:** estruturou os contratos `Repository` e `ObjectStore` com adapters em memória e reais, implementou o protocolo S3 Signature V4 em Go puro (evitando dependências transitivas que poderiam quebrar o gate do `govulncheck` no CI), garantiu idempotência no importador e validou os cenários `INT-04`, `INT-05` e `RES-05` com testes automatizados. **Precisou ser corrigido:** no ambiente Docker Compose local o navegador acessa `localhost` e não resolve o host interno `http://minio:9000` de uma URL pré-assinada direta; por isso o handler `/musicas/{filename}` foi ajustado para suportar streaming proxy direto do MinIO por padrão (sem usar disco local) além de URL pré-assinada (`?presign=1` / `AUDIO_DELIVERY_MODE=presigned`), e as regras do Traefik precisaram excluir explicitamente as sub-rotas `/(upload|youtube|cookies.*)` do router `station-api` para não interceptar as rotas da frente de mídia |
| Claude (Anthropic), via Claude Code — Davi Alves (Membro 2) | Entrega 3, frente do Media Service + Worker (#16): Media API (`cmd/media`), Media Worker (`cmd/worker`), fila no RabbitMQ com retry e DLQ (`internal/queue`), andamento e locks dos jobs no Redis (`internal/jobs`), circuit breaker (`internal/breaker`), métricas, estado "processando…" no painel do DJ, Dockerfiles, compose, CI, testes INT-02, RES-02 e RES-03 e a atualização do ADR-0002. Na Entrega 2, ajudou a abrir o PR da frente (aplicar o patch, rodar os testes e escrever a descrição) | Recebeu a issue #16 e o repositório; leu o código das outras frentes e implementou a issue inteira numa sessão. Subiu a stack com Docker Compose e testou com clientes HTTP e WebSocket, navegador headless, `docker kill` e um vídeo real do YouTube | **Funcionou:** os testes na stack real acharam um erro que os testes em memória escondiam (`AddTrackToStation` falhava sempre no Postgres). A leitura do código mostrou que o upload do monolito já não enxergava as estações do Station Service. Defeitos introduzidos de propósito no worker foram todos detectados pelos testes. **Precisou ser corrigido:** a primeira versão do cliente AMQP escolhida tinha CVEs conhecidas, apontadas pelo govulncheck e pelo Trivy (hoje 1.16.0, sem achados); o lock do job começou com 30 s e a recuperação de um worker morto levava 36 s (hoje 22 s, com 15 s); um teste do circuit breaker não exercitava o caso que dizia exercitar; e o tempo de `202` não vale para arquivos muito grandes, porque a transferência domina (registrado no ADR-0002) |

---

## 📄 Licença

Este projeto é de código aberto e está disponível sob a licença MIT.
