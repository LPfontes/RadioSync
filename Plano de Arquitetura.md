# **Plano de Projeto: Plataforma de Rádio Web Sincronizada (RadioSync)**

## **1. Visão Geral do Projeto**

Este documento descreve a arquitetura e o planejamento da **RadioSync**, uma plataforma web de rádio interativa de escuta sincronizada. A premissa central é permitir que usuários criem "estações" (salas) onde um DJ controla a playlist, o mixer de efeitos sonoros e a reprodução, enquanto todos os ouvintes conectados escutam a mesma música simultaneamente, podendo enviar sugestões em tempo real. Para minimizar os custos e a carga no servidor, a transferência do áudio é feita por download estático no cliente, enquanto a sincronização do estado da reprodução é mantida via WebSockets.

---

## **2. Arquitetura do Sistema**

### **2.1 Arquitetura Atual (Monolito Modular)**

> * **Frontend (Vue 3 + Vanilla CSS):** Single Page Application construída com Vue 3 (Composition API), Vite, Pinia e estilização em Vanilla CSS com layout inspirado no YouTube Music (player fixo inferior, suporte a capas/thumbnails, grid de músicas e *drag-and-drop* na playlist). Gerencia a reprodução de áudio localmente via HTML5 Audio API, o mixer de áudio ambiente e responde aos eventos recebidos pelo WebSocket.
> * **Backend (Go):** Servidor concorrente que gerencia a API REST (`/api/v1`), as conexões WebSocket por estação (`gorilla/websocket`), a transcodificação de áudio com **FFmpeg**, a importação de faixas do YouTube via **yt-dlp** (integrado ao `bgutil-ytdlp-pot-provider`) e a persistência atômica em disco.
> * **Formato de Áudio (Opus):** Todas as faixas são transcodificadas e servidas no formato `.opus` (container Ogg, 96 kbps VBR). Isso garante alta fidelidade sonora com baixo consumo de banda e transições suaves entre as músicas da playlist (ADR-0004).

### **2.2 Evolução para Arquitetura Distribuída (Entregas 2 e 3)**

Para escalar horizontalmente conexões WebSocket, isolar o uso intensivo de CPU do FFmpeg/yt-dlp e eliminar pontos únicos de falha, o sistema evolui incrementalmente por meio do padrão **Strangler Fig** ([ADR-0003](docs/adr/0003-migracao-strangler-fig.md)) com os seguintes containers ([C4 Nível 2](docs/c4/02-containers.md)):

| Componente | Tecnologia | Papel na Arquitetura | ADR |
| :--- | :--- | :--- | :--- |
| **API Gateway / Load Balancer** | Traefik v3 | Ponto único de entrada, roteamento incremental (Strangler Fig), *rate limiting* e métricas. | [ADR-0003](docs/adr/0003-migracao-strangler-fig.md), [ADR-0005](docs/adr/0005-api-gateway-seguranca.md) |
| **Sync Service (N réplicas)** | Go + WebSocket | Mantém conexões WebSocket dos ouvintes e realiza *fan-out* de eventos entre réplicas via Redis Pub/Sub. | [ADR-0001](docs/adr/0001-redis-pubsub-fanout-websocket.md), [ADR-0004](docs/adr/0004-sincronizacao-client-side-timestamp.md) |
| **Media API + Media Worker** | Go + FFmpeg + yt-dlp | Desacopla upload e download do YouTube em jobs assíncronos via fila, com *Retry*, *DLQ* e *Circuit Breaker*. | [ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md) |
| **Station Service** | Go | CRUD de estações, playlist, biblioteca global de faixas e emissão/validação de tokens JWT. | [ADR-0006](docs/adr/0006-persistencia-postgres-minio.md) |
| **Estado Quente + Mensageria** | Redis 7 | Armazena o `PlaybackState` ativo de baixa latência e opera os canais Pub/Sub `station:{id}:events`. | [ADR-0001](docs/adr/0001-redis-pubsub-fanout-websocket.md) |
| **Fila de Transcodificação** | RabbitMQ 3.13 | Fila `media.transcode` e Dead-Letter Queue (`media.transcode.dlq`) com confirmação manual (*ack*). | [ADR-0002](docs/adr/0002-transcodificacao-assincrona-fila.md) |
| **Persistência Durável e Objetos** | PostgreSQL 16 + MinIO (S3) | Postgres armazena estações e catálogo relacional de faixas; MinIO armazena os arquivos `.opus` compartilhados. | [ADR-0006](docs/adr/0006-persistencia-postgres-minio.md) |
| **Observabilidade** | Prometheus + Grafana | Coleta de métricas de latência (p95), conexões WebSocket ativas, taxa de erros e profundidade de fila. | — |

---

## **3. Estrutura de Dados e Estado**

O servidor mantém e sincroniza as seguintes estruturas principais (`backend/internal/model`):

| Estrutura / Entidade | Descrição | Principais Atributos |
| :--- | :--- | :--- |
| **PlaybackState** | Define o status exato da música em execução. | `IsPlaying` (booleano), `StartedAt` (timestamp Unix em ms), `SeekOffset` (segundos acumulados), `CurrentSong` (URL da música `.opus`), `Duration` (segundos). |
| **Track** | Representa uma faixa de áudio convertida. | `ID` (UUID), `Title`, `URL` (`/musicas/<uuid>.opus`), `Filename`, `Duration`, `Thumbnail` (capa da faixa / miniatura do YouTube). |
| **SongSuggestion** | Sugestão enviada por ouvintes para o DJ. | `ID`, `TrackID`, `Title`, `URL`, `SuggestedBy`, `CreatedAt`, `Status` (`pending`, `approved`, `rejected`). |
| **MixerChannel** | Canal do mixer de efeitos sonoros/ambiente. | `ID`, `Name`, `Type` (`preset` ou `track`), `PresetKey` (`rain`, `thunder`, `cafe`, `fireplace`, `waves`, `wind`, `forest`, etc.), `TrackURL`, `Volume`, `IsPlaying`, `Muted`, `Loop`. |
| **Station** | Representa uma sala ativa na plataforma. | `ID` (código curto de 6 caracteres), `DJ`, `Hub` (gerenciador WebSocket), `State` (`PlaybackState`), `Repository`, `Playlist`, `Suggestions`, `Mixer`. |

### **3.1 Gestão de Repositório, Biblioteca Global e Playlist**

> * **Repositório da Estação:** Contém as músicas enviadas (via upload ou YouTube) para aquela estação.
> * **Biblioteca Global (`/api/v1/library`):** Catálogo unificado de todas as faixas `.opus` disponíveis no servidor, permitindo que qualquer estação reutilize músicas já convertidas sem retrabalho de transcodificação.
> * **Playlist:** A fila de reprodução ativa da estação, suportando adição, remoção, reprodução imediata de faixa específica e reordenação via *drag-and-drop*.

### **3.2 Persistência de Dados**

> * **Situação Atual (`handler/persist.go`):** Salvamento periódico (a cada 30s) e por eventos das estações em `stations.json`, catálogo de nomes em `track_names.json` e cache de downloads do YouTube em `yt_cache.json`, utilizando escrita em arquivo temporário seguida de substituição atômica (*atomic rename*).
> * **Arquitetura Distribuída ([ADR-0006](docs/adr/0006-persistencia-postgres-minio.md)):** Migração para **Polyglot Persistence** com Redis (estado quente do player), PostgreSQL (estações e catálogo relacional de faixas) e MinIO (armazenamento S3 dos arquivos `.opus`).

---

## **4. Sincronização de Áudio Client-Side ([ADR-0004](docs/adr/0004-sincronizacao-client-side-timestamp.md))**

Para garantir que todos os ouvintes escutem a música no mesmo instante sem trafegar stream contínuo de áudio pelo servidor, o cálculo de posição é feito no cliente (Vue) com base em timestamps:

> 1. O DJ aciona `PLAY` ou `SEEK`, e o servidor Go registra o timestamp exato (`StartedAt = time.Now().UnixMilli()`) junto ao `SeekOffset`, transmitindo o `STATE_UPDATE` via WebSocket.
> 2. O cliente calcula a posição esperada da faixa:
>    $$\text{Posição Atual} = \frac{\text{Timestamp Atual} - \text{StartedAt}}{1000} + \text{SeekOffset}$$
> 3. O cliente também pode solicitar sincronização pontual enviando `SYNC_REQUEST`, recebendo do servidor a posição exata calculada por `calcPosition()` e o estado atual do mixer.
> 4. Se a diferença (*drift*) entre a posição calculada e `audio.currentTime` ultrapassar a tolerância (1,5s), o player corrige `audio.currentTime` automaticamente.

---

## **5. Implementação Técnica Principal**

### **5.1 Configuração Go (Backend)**

O servidor registra o MIME type `.opus` como `audio/ogg` antes de inicializar o servidor de arquivos e configura o roteador `chi` com suporte a CORS, recuperação de panics e rotas REST/WebSocket (`backend/cmd/server/main.go`).

### **5.2 Configuração Vue 3 (Frontend)**

- Rota unificada `/radio/:stationId`: diferencia automaticamente ouvinte e DJ com base na presença e validade do `djToken` salvo no `localStorage`. Caso não haja DJ online na sala, qualquer usuário pode assumir a bancada com 1 clique (`POST /api/v1/stations/{stationId}/claim-dj`).
- Player fixo inferior estilo YouTube Music (`Player.vue`) e mixer de efeitos sonoros (`AudioMixer.vue`) sincronizado em tempo real com todos os ouvintes da estação.

### **5.3 Conversão de Mídia e Importação do YouTube**

- **FFmpeg (`internal/media/convert.go`):** Aceita uploads nos formatos `.mp3`, `.mp4`, `.wav`, `.ogg`, `.m4a`, `.flac`, `.aac`, `.webm` e `.opus`, convertendo-os para Opus a `96k` VBR e extraindo a duração exata via `ffprobe`.
- **YouTube (`internal/media/youtube.go`):** Importa áudios do YouTube via `yt-dlp` integrado ao provedor de PO Token (`bgutil-ytdlp-pot-provider`), mantendo cache por ID de vídeo (`yt_cache.json`) para evitar downloads e conversões duplicadas e extraindo metadados e miniaturas (capas).

### **5.4 Otimização de Tráfego: Janela Deslizante (Top 3)**

- **Filtro de Payload:** Nas atualizações de playlist enviadas aos ouvintes, o backend envia a janela das próximas faixas para permitir *pre-buffer* sem sobrecarregar a banda.
- **Transição Automática:** Quando a faixa atual termina (monitorada por *ticker* no servidor e eventos no cliente), a fila avança automaticamente (`NEXT_TRACK` / `TRACK_CHANGED`) e a próxima música entra em reprodução.

---

## **6. Planejamento da API HTTP e WebSockets**

### **6.1 Rotas de Estações e Biblioteca (`/api/v1`)**

> * **`POST /api/v1/stations`:** Cria uma nova estação com código curto de 6 caracteres. Retorna `stationId` e `djToken` (JWT HS256).
> * **`GET /api/v1/stations/{stationId}`:** Retorna metadados da estação, estado do player, playlist, repositório, sugestões, mixer e se há um DJ conectado (`hasDJOnline`).
> * **`POST /api/v1/stations/{stationId}/claim-dj`:** Permite assumir o papel de DJ emitindo um novo `djToken` quando nenhum DJ estiver conectado na sala.
> * **`GET /api/v1/library`** e **`GET /api/v1/stations/{stationId}/library`:** Retorna o catálogo global de músicas disponíveis no servidor.

### **6.2 Rotas Protegidas do DJ (`Authorization: Bearer <djToken>`)**

> * **`POST /api/v1/stations/{stationId}/upload`:** Upload multipart de arquivo de áudio e conversão para `.opus`.
> * **`POST /api/v1/stations/{stationId}/youtube`:** Download e conversão de áudio a partir de uma URL do YouTube.
> * **`GET /api/v1/stations/{stationId}/repository`:** Lista todas as faixas do repositório da estação.
> * **`PUT /api/v1/stations/{stationId}/tracks/{trackId}`:** Renomeia ou atualiza metadados de uma faixa.
> * **`POST /api/v1/stations/{stationId}/cookies`** e **`GET .../cookies/status`:** Gerenciamento de cookies para autenticação do `yt-dlp`.

### **6.3 Rotas Administrativas (`/api/v1/admin`)**

> * **`GET /api/v1/admin/stations`** e **`DELETE /api/v1/admin/stations/{stationId}`:** Listagem e exclusão de estações.
> * **`POST /api/v1/admin/purge-orphans`:** Remove arquivos `.opus` órfãos que não estão vinculados a nenhuma estação.
> * **`PUT /api/v1/admin/tracks/{trackId}`** e **`DELETE /api/v1/admin/stations/{stationId}/tracks/{trackId}`:** Gestão administrativa de faixas.

### **6.4 Conexão e Eventos WebSocket (`/ws/stations/{stationId}`)**

Aceita `?token=<djToken>` para autenticar o DJ (caso não haja outro DJ já conectado).

- **Comandos de Ouvinte/Geral:** `SYNC_REQUEST`, `SUGGEST_TRACK`.
- **Comandos Exclusivos do DJ:** `PLAY`, `PAUSE`, `SEEK`, `NEXT_TRACK`, `ADD_TO_PLAYLIST`, `PLAY_PLAYLIST_TRACK`, `REMOVE_FROM_PLAYLIST`, `REORDER_PLAYLIST`, `APPROVE_SUGGESTION`, `REJECT_SUGGESTION`, `CLEAR_SUGGESTIONS`, `MIXER_UPDATE`, `MIXER_SET_CHANNEL`, `MIXER_REMOVE_CHANNEL`, `MIXER_CLEAR`.
- **Eventos de Broadcast do Servidor:** `STATE_UPDATE`, `TRACK_CHANGED`, `PLAYLIST_UPDATED`, `SUGGESTIONS_UPDATED`, `MIXER_UPDATED`, `DJ_STATUS`, `SYNC`.

---

## **7. Stack Tecnológico e Pacotes**

Consulte também [`docs/stack.md`](docs/stack.md) para as justificativas detalhadas.

### **7.1 Backend (Go 1.26)**

* **[`github.com/gorilla/websocket`](https://github.com/gorilla/websocket):** Upgrade HTTP e gerenciamento das conexões WebSocket com Ping/Pong e deadlines.
* **[`github.com/go-chi/chi/v5`](https://github.com/go-chi/chi/v5):** Roteador HTTP leve e middlewares (`/api/v1/`).
* **[`github.com/rs/cors`](https://github.com/rs/cors):** Configuração de Cross-Origin Resource Sharing.
* **[`github.com/google/uuid`](https://github.com/google/uuid):** Geração de UUIDs para faixas, clientes e sugestões.
* **[`github.com/golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt/v5):** Emissão e validação stateless do `djToken` (HS256).
* **[`github.com/joho/godotenv`](https://github.com/joho/godotenv):** Carregamento de variáveis de ambiente.

### **7.2 Frontend (Vue 3)**

* **`vue` (v3) + `vite`:** SPA reativa com Composition API e build otimizado.
* **`vue-router` (v4):** Roteamento das telas (`/`, `/radio/:stationId`, `/admin`).
* **`pinia`:** Gerenciamento de estado reativo sincronizado ao WebSocket.
* **`axios`:** Cliente HTTP para chamadas REST e uploads.
* **Vanilla CSS (`src/style.css`):** Design system próprio tema escuro estilo YouTube Music, sem dependência de Tailwind.
* **`lucide-vue-next`:** Ícones SVG da interface.

### **7.3 Infraestrutura, Mensageria, Dados e Observabilidade**

* **Docker & Docker Compose:** Orquestração local da aplicação e de todos os serviços de infraestrutura.
* **Traefik v3:** API Gateway, descoberta automática de containers, *rate limiting* e exportação de métricas.
* **Redis 7:** Estado quente das estações e Pub/Sub para fan-out de WebSockets.
* **RabbitMQ 3.13:** Fila de mensagens para transcodificação assíncrona e Dead-Letter Queue (DLQ).
* **PostgreSQL 16 & MinIO:** Banco de dados relacional e Object Storage compatível com S3.
* **Prometheus & Grafana:** Coleta de séries temporais e visualização de métricas.
* **GitHub Actions:** Pipeline de CI executando `go vet`, `go test -race ./...`, build do frontend, validação do Compose e build da imagem Docker.

---

## **8. Roadmap e Fases de Desenvolvimento**

| Fase | Objetivo | Entregáveis | Status |
| :--- | :--- | :--- | :--- |
| **Fase 1: MVP** | Conexão e sincronização básica. | Servidor Go servindo `.opus`; Hub WebSocket; player Vue sincronizado por timestamp. | Concluído |
| **Fase 2: Gestão de Salas e DJ** | Fluxo multi-usuário e autenticação. | `PlaybackState`, Repositório, Playlist, JWT (`djToken`), upload e transcodificação FFmpeg. | Concluído |
| **Fase 3: Otimizações e Retenção** | Experiência de escuta contínua. | Janela deslizante (Top 3 + pre-buffer), Ping/Pong com timeouts, desbloqueio de Autoplay. | Concluído |
| **Fase 4: Recursos Avançados e Nova UI** | Interatividade completa e persistência. | Importação do YouTube (`yt-dlp` + PO Token), Biblioteca Global, Sugestões de ouvintes, Mixer de efeitos sonoros sincronizado, Assumir DJ com 1 clique, Painel Admin, Persistência atômica JSON e UI Vanilla CSS estilo YouTube Music com capas e *drag-and-drop*. | Concluído |
| **Fase 5: Arquitetura Distribuída (Entrega 2)** | Documentação arquitetural, infra e CI. | Diagramas C4 (Níveis 1 e 2), ADRs 0001–0006, `docker-compose.yml` completo (Traefik, Redis, RabbitMQ, Postgres, MinIO, Prometheus, Grafana), CI no GitHub Actions e testes unitários com `-race`. | Concluído |
| **Fase 6: Extração e Resiliência (Entrega 3)** | Escalabilidade horizontal e testes de carga. | Extração via Strangler Fig do Sync Service (Redis Pub/Sub), Media Worker (RabbitMQ + Retry/DLQ + Circuit Breaker) e Station Service (Postgres + MinIO), endpoint `/metrics`, dashboards Grafana e testes de carga/resiliência com k6. | Em andamento |
