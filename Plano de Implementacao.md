# Plano de Implementação — RadioSync (Rádio Web Sincronizada)

## 1. Estrutura de Diretórios Atual

```text
radio/
├── .github/
│   └── workflows/
│       └── ci.yml                  # Pipeline CI (go vet, go test -race, build frontend, compose e docker)
├── backend/                        # Servidor Go (1.26)
│   ├── cmd/
│   │   └── server/
│   │       └── main.go             # Entry point HTTP, rotas REST/WS e arquivos estáticos
│   ├── internal/
│   │   ├── auth/                   # Autenticação JWT (HS256)
│   │   │   ├── jwt.go              # Geração e validação do djToken por estação
│   │   │   └── jwt_test.go         # Testes unitários de autenticação e segurança JWT
│   │   ├── handler/                # HTTP & WebSocket handlers
│   │   │   ├── admin.go            # Rotas administrativas (/api/v1/admin/*)
│   │   │   ├── debug.go            # Diagnóstico de ferramentas (ffmpeg, ffprobe, yt-dlp, deno)
│   │   │   ├── musicas.go          # Listagem da biblioteca global e catálogo de faixas
│   │   │   ├── persist.go          # Persistência atômica (stations.json, track_names.json, yt_cache.json)
│   │   │   ├── station.go          # Criação/consulta de estações, claim-dj e códigos de 6 caracteres
│   │   │   ├── station_test.go     # Testes unitários de geração de código e extração de ID do YouTube
│   │   │   ├── upload.go           # Upload multipart, conversão para Opus e edição de metadados
│   │   │   ├── websocket.go        # Upgrade HTTP -> WS, validação de papel (dj/listener)
│   │   │   └── youtube.go          # Importação do YouTube via yt-dlp e gestão de cookies
│   │   ├── media/                  # Processamento de áudio e mídia externa
│   │   │   ├── convert.go          # Wrapper FFmpeg (Opus 96k VBR) e ffprobe (duração)
│   │   │   ├── convert_test.go     # Testes unitários de formatos de áudio aceitos/rejeitados
│   │   │   └── youtube.go          # Wrapper yt-dlp + bgutil PO Token Provider + extração de thumbnail
│   │   ├── model/                  # Estruturas de domínio e máquina de estados da estação
│   │   │   ├── station.go          # Station, PlaybackState, SongSuggestion, MixerChannel e comandos WS
│   │   │   ├── station_test.go     # Testes de calcPosition e ExtractYouTubeURL
│   │   │   └── track.go            # Struct Track (ID, Title, URL, Filename, Duration, Thumbnail)
│   │   └── ws/                     # Gerenciamento de conexões WebSocket
│   │       ├── client.go           # Conexão individual, ReadPump/WritePump e Ping/Pong
│   │       ├── hub.go              # Hub por estação, Broadcast, Register/Unregister e DJ_STATUS
│   │       └── hub_test.go         # Testes unitários de fan-out e detecção de DJ conectado
│   ├── go.mod
│   └── go.sum
├── frontend/                       # SPA Vue 3 + Vite + Pinia (Vanilla CSS)
│   ├── public/
│   │   └── sounds/                 # Presets de áudio do mixer (rain, thunder, cafe, fireplace, waves, etc.)
│   ├── src/
│   │   ├── components/
│   │   │   ├── AudioMixer.vue      # Mixer de efeitos sonoros e ambiente sincronizado
│   │   │   └── Player.vue          # Player fixo estilo YouTube Music com capa e controles
│   │   ├── composables/
│   │   │   ├── usePlayer.js        # Sincronização client-side por timestamp e pre-buffer
│   │   │   └── useWebSocket.js     # Conexão WS, reconexão automática e envio de comandos
│   │   ├── router/
│   │   │   └── index.js            # Rotas: /, /radio/:stationId (unificada DJ/Ouvinte) e /admin
│   │   ├── services/
│   │   │   ├── ambientAudio.js     # Reprodução Web Audio / HTML5 dos canais do mixer
│   │   │   ├── ambientPresets.js   # Catálogo de presets sonoros
│   │   │   ├── api.js              # Instância Axios para a API REST
│   │   │   └── storage.js          # Persistência local (histórico de salas e tokens no localStorage)
│   │   ├── stores/
│   │   │   └── station.js          # Store Pinia (estado do player, playlist, repositório, sugestões, mixer)
│   │   ├── views/
│   │   │   ├── Admin.vue           # Painel administrativo de estações e faixas
│   │   │   ├── DJDashboard.vue     # Interface de controle do DJ (grid, drag-and-drop, sugestões, YouTube)
│   │   │   ├── Home.vue            # Criação de estação, entrada por código e salas recentes
│   │   │   └── Listen.vue          # Visão da estação (unifica experiência de ouvinte e bancada do DJ)
│   │   ├── App.vue
│   │   ├── main.js
│   │   └── style.css               # Design system em Vanilla CSS (tema escuro estilo YouTube Music)
│   ├── index.html
│   ├── vite.config.js
│   └── package.json
├── docs/                           # Documentação de Arquitetura (Entrega 2)
│   ├── adr/                        # Architecture Decision Records (0000 a 0006)
│   ├── c4/                         # Diagramas C4 em Mermaid (01-contexto.md, 02-containers.md)
│   ├── divisao-responsabilidades.md
│   ├── padroes.md                  # Padrões arquiteturais aplicados
│   ├── plano-de-testes.md          # Estratégia de testes unitários, integração, carga (k6) e resiliência
│   └── stack.md                    # Stack tecnológico e justificativas
├── infra/                          # Configurações de Observabilidade
│   ├── grafana/provisioning/datasources/prometheus.yml
│   └── prometheus/prometheus.yml
├── musicas/                        # Diretório local para arquivos .opus convertidos (ignorado no git)
├── Dockerfile                      # Build multi-stage (Frontend Node + Backend Go + FFmpeg + yt-dlp + Deno)
├── docker-compose.yml              # Stack completa (App, Traefik, Redis, RabbitMQ, Postgres, MinIO, Prometheus, Grafana)
└── .env.example                    # Template de variáveis de ambiente
```

---

## 2. Ordem de Implementação (Faseado)

### FASE 1 — MVP: Conexão e Sincronização Básica (Concluída)

| # | Ação | Arquivos | Detalhes |
|---|---|---|---|
| 1.1 | Inicializar módulo Go | `backend/go.mod` | `go mod init radio-backend` |
| 1.2 | Criar `main.go` com servidor HTTP + MIME Opus | `backend/cmd/server/main.go` | Servir `./musicas` em `/musicas/` |
| 1.3 | Adicionar dependências Go | `go.mod` | `gorilla/websocket`, `go-chi/chi/v5`, `rs/cors`, `google/uuid`, `golang-jwt/jwt/v5`, `joho/godotenv` |
| 1.4 | Criar modelos de dados | `backend/internal/model/station.go`, `track.go` | Structs `Station`, `PlaybackState`, `Track` |
| 1.5 | Criar gerenciamento WebSocket (Hub + Client) | `backend/internal/ws/hub.go`, `client.go` | Hub por `stationId`, Ping/Pong, broadcast |
| 1.6 | Criar handler WebSocket | `backend/internal/handler/websocket.go` | Upgrade HTTP -> WS, registrar cliente no Hub |
| 1.7 | Criar handler de estações | `backend/internal/handler/station.go` | `POST/GET /api/v1/stations` |
| 1.8 | Roteador Chi + CORS | `backend/cmd/server/main.go` | Configurar rotas e middleware |
| 1.9 | Inicializar frontend Vite + Vue 3 | `frontend/` | SPA com Vue Router e Pinia |
| 1.10 | Criar `useWebSocket` e `usePlayer` | `frontend/src/composables/` | Conexão WS, reconexão e sincronização de áudio por `StartedAt` e `SeekOffset` |
| 1.11 | Criar `Player.vue` e views iniciais | `frontend/src/components/Player.vue`, `views/` | Tag `<audio>` com Opus, `Home.vue` e `Listen.vue` |

### FASE 2 — Gestão de Salas e Fluxo do DJ (Concluída)

| # | Ação | Arquivos | Detalhes |
|---|---|---|---|
| 2.1 | Implementar JWT | `backend/internal/auth/jwt.go` | Gerar e validar `djToken` atrelado ao `stationId` |
| 2.2 | Proteger ações do DJ | `backend/internal/handler/*` | Validação Bearer Token nas rotas HTTP e query param `?token=` no WS |
| 2.3 | Implementar upload + conversão | `backend/internal/media/convert.go`, `handler/upload.go` | Receber multipart, converter para `.opus` via FFmpeg |
| 2.4 | Rota `GET /repository` | `backend/internal/handler/upload.go` | Listar músicas do repositório da estação |
| 2.5 | Comandos WS de reprodução e fila | `backend/internal/model/station.go` | `PLAY`, `PAUSE`, `SEEK`, `NEXT_TRACK`, `ADD_TO_PLAYLIST`, `REMOVE_FROM_PLAYLIST` |
| 2.6 | Auto-avanço de faixa | `backend/internal/model/station.go` | Goroutine com ticker verificando fim da música pela duração |

### FASE 3 — Otimizações (Concluída)

| # | Ação | Arquivos | Detalhes |
|---|---|---|---|
| 3.1 | Janela deslizante (Top 3) | `backend/internal/model/station.go` | Enviar apenas as próximas faixas para os ouvintes |
| 3.2 | Pre-buffer no frontend | `frontend/src/composables/usePlayer.js` | Pré-carregamento da próxima música da fila |
| 3.3 | Extrair duração com `ffprobe` | `backend/internal/media/convert.go` | Obter duração precisa após conversão |
| 3.4 | Ping/Pong com deadlines | `backend/internal/ws/client.go` | Detecção e limpeza de conexões inativas |

### FASE 4 — Recursos Avançados, Persistência e Nova UI (Concluída)

| # | Ação | Arquivos | Detalhes |
|---|---|---|---|
| 4.1 | Importação do YouTube + Cache | `internal/media/youtube.go`, `handler/youtube.go` | Download via `yt-dlp` + `bgutil-ytdlp-pot-provider`, extração de capa e cache em `yt_cache.json` |
| 4.2 | Persistência atômica em disco | `backend/internal/handler/persist.go` | Salvamento periódico e por evento (`stations.json`, `track_names.json`) via temp file + rename |
| 4.3 | Biblioteca Global de músicas | `backend/internal/handler/musicas.go` | Catálogo compartilhado entre todas as estações (`GET /api/v1/library`) |
| 4.4 | Sistema de Sugestões de Ouvintes | `backend/internal/model/station.go` | `SUGGEST_TRACK`, `APPROVE_SUGGESTION` (com download automático se for link do YouTube), `REJECT_SUGGESTION` |
| 4.5 | Mixer de Efeitos Sonoros Sincronizado | `frontend/src/components/AudioMixer.vue`, `services/ambientAudio.js` | Canais de som ambiente (chuva, trovão, cafeteria, lareira, etc.) sincronizados via WS (`MIXER_*`) |
| 4.6 | Assumir como DJ com 1 clique | `backend/internal/handler/station.go` | Rota `POST /api/v1/stations/{id}/claim-dj` e broadcast de `DJ_STATUS` (`hasDJOnline`) |
| 4.7 | Unificação de rota e UI estilo YouTube Music | `frontend/src/style.css`, `views/Listen.vue`, `Player.vue` | Migração de Tailwind para Vanilla CSS, rota única `/radio/:stationId`, grid com capas, player fixo e *drag-and-drop* (`REORDER_PLAYLIST`) |
| 4.8 | Painel Administrativo | `backend/internal/handler/admin.go`, `frontend/src/views/Admin.vue` | Gestão de salas, renomeação de faixas, upload de cookies e limpeza de arquivos órfãos |

### FASE 5 — Arquitetura Distribuída, Infraestrutura e Testes (Entrega 2 — Concluída)

| # | Ação | Arquivos | Detalhes |
|---|---|---|---|
| 5.1 | Modelagem C4 (Níveis 1 e 2) | `docs/c4/01-contexto.md`, `02-containers.md` | Diagramas C4 em Mermaid (arquitetura atual vs. arquitetura-alvo distribuída) |
| 5.2 | Decisões Arquiteturais (ADRs) | `docs/adr/0000` a `0006` | Redis Pub/Sub (0001), Fila RabbitMQ/Worker (0002), Strangler Fig (0003), Sync Client-Side (0004), API Gateway Traefik (0005) e Postgres+MinIO (0006) |
| 5.3 | Ambiente Completo Docker Compose | `docker-compose.yml`, `.env.example` | Orquestração de `radiosync`, `bgutil-provider`, `gateway` (Traefik v3.6), `redis`, `rabbitmq`, `postgres`, `minio`, `prometheus` e `grafana` |
| 5.4 | Observabilidade inicial | `infra/prometheus/`, `infra/grafana/` | Scrape config do Prometheus e provisionamento automático de datasource no Grafana |
| 5.5 | Testes Unitários e Correção de Bugs | `backend/internal/**/*_test.go` | Testes automatizados para `auth`, `model`, `handler`, `media` e `ws` (incluindo correção de bug em `ExtractYouTubeURL`) |
| 5.6 | Pipeline de CI | `.github/workflows/ci.yml` | Execução automática de `go vet`, `go test -race ./...`, build do frontend, validação do Compose e build Docker |

### FASE 6 — Extração de Serviços e Testes de Carga/Resiliência (Entrega 3 — Planejada)

| # | Ação | Frente Responsável | Detalhes |
|---|---|---|---|
| 6.1 | **Sync Service** escalável | Membro 1 (Sync) | Fan-out via Redis Pub/Sub (`station:{id}:events`), estado quente no Redis e múltiplas réplicas |
| 6.2 | **Media Service + Worker** | Membro 2 (Media) | Publicação na fila RabbitMQ (`media.transcode`), worker idempotente, 3 retries com backoff exponencial, DLQ e Circuit Breaker no `yt-dlp` |
| 6.3 | **Segurança de Borda e Deploy** | Membro 3 (Infra/Segurança) | Proteção das rotas `/api/v1/admin/*`, CORS restrito, regras de rate limit por rota no Traefik e scan de segurança no CI |
| 6.4 | **Station Service + Persistência Poliglota** | Membro 4 (Dados) | Migração do `stations.json` para PostgreSQL 16 e armazenamento de `.opus` no MinIO (S3) com URLs pré-assinadas |
| 6.5 | **Observabilidade e Testes de Carga** | Membro 5 (Testes/Obs.) | Endpoint `/metrics`, dashboards no Grafana, scripts de carga **k6** e execução dos testes de resiliência (`docs/plano-de-testes.md`) |

---

## 3. Comandos de Execução e Testes

### Ambiente Completo via Docker Compose (Recomendado)
```bash
cp .env.example .env
docker compose up -d --build
```

### Desenvolvimento Local
```bash
# Backend (porta 8080)
cd backend
go run ./cmd/server/main.go

# Frontend (porta 5173)
cd frontend
npm install
npm run dev
```

### Execução dos Testes Automatizados
```bash
cd backend
go test -v -race ./...
```

---

## 4. Protocolo WebSocket (Mensagens JSON)

### Do Cliente (Ouvinte ou DJ) para o Servidor
```json
{"type": "SYNC_REQUEST"}
{"type": "SUGGEST_TRACK", "data": {"trackId": "uuid", "title": "Nome ou URL YouTube", "url": "", "suggestedBy": "Nome"}}
```

### Exclusivos do DJ para o Servidor
```json
{"type": "PLAY"}
{"type": "PAUSE"}
{"type": "SEEK", "data": {"position": 45.5}}
{"type": "NEXT_TRACK"}
{"type": "ADD_TO_PLAYLIST", "data": {"trackId": "uuid-da-faixa"}}
{"type": "PLAY_PLAYLIST_TRACK", "data": {"trackId": "uuid-da-faixa"}}
{"type": "REMOVE_FROM_PLAYLIST", "data": {"trackId": "uuid-da-faixa"}}
{"type": "REORDER_PLAYLIST", "data": {"fromIndex": 2, "toIndex": 0}}
{"type": "APPROVE_SUGGESTION", "data": {"suggestionId": "uuid-da-sugestao"}}
{"type": "REJECT_SUGGESTION", "data": {"suggestionId": "uuid-da-sugestao"}}
{"type": "CLEAR_SUGGESTIONS"}
{"type": "MIXER_UPDATE", "data": {"channels": []}}
{"type": "MIXER_SET_CHANNEL", "data": {"channel": {"id": "rain", "name": "Chuva", "type": "preset", "presetKey": "rain", "volume": 0.35, "isPlaying": true, "muted": false, "loop": true}}}
{"type": "MIXER_REMOVE_CHANNEL", "data": {"id": "rain"}}
{"type": "MIXER_CLEAR"}
```

### Do Servidor para os Clientes (Broadcast / Resposta Direta)
```json
{"type": "STATE_UPDATE", "state": {"isPlaying": true, "startedAt": 1712345678000, "seekOffset": 0, "currentSong": "/musicas/uuid.opus", "duration": 180.5}}
{"type": "TRACK_CHANGED", "playlist": []}
{"type": "PLAYLIST_UPDATED", "playlist": []}
{"type": "SUGGESTIONS_UPDATED", "suggestions": []}
{"type": "MIXER_UPDATED", "channels": []}
{"type": "DJ_STATUS", "hasDJOnline": true}
{"type": "SYNC", "position": 42.15, "mixer": []}
```

---

## 5. Decisões Técnicas Consolidadas

- **Sincronização Client-Side ([ADR-0004](docs/adr/0004-sincronizacao-client-side-timestamp.md)):** Áudio estático em `.opus` (96 kbps VBR) servido por HTTP e sincronizado via eventos JSON leves no WebSocket.
- **Escalabilidade de WebSockets ([ADR-0001](docs/adr/0001-redis-pubsub-fanout-websocket.md)):** Fan-out de eventos entre múltiplas instâncias do Sync Service via Redis Pub/Sub.
- **Processamento Assíncrono de Mídia ([ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md)):** Fila RabbitMQ com workers dedicados para FFmpeg/yt-dlp, com 3 tentativas (backoff exponencial), Dead-Letter Queue (`media.transcode.dlq`) e Circuit Breaker.
- **Migração Incremental ([ADR-0003](docs/adr/0003-migracao-strangler-fig.md)):** Adoção do padrão *Strangler Fig* mediado pelo API Gateway Traefik.
- **Segurança e Rate Limiting ([ADR-0005](docs/adr/0005-api-gateway-seguranca.md)):** Traefik na borda aplicando *rate limiting*, JWT HS256 por estação para o DJ e autenticação para rotas administrativas.
- **Persistência Poliglota ([ADR-0006](docs/adr/0006-persistencia-postgres-minio.md)):** Substituição gradual dos arquivos JSON locais por Redis (estado em tempo real), PostgreSQL (metadados relacionais) e MinIO (objetos de áudio `.opus`).
