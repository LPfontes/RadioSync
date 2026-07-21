# Plano de Implementação - Rádio Web Sincronizada

## 1. Estrutura de Diretórios

```
radio/
├── backend/                    # Servidor Go
│   ├── cmd/
│   │   └── server/
│   │       └── main.go         # Entry point
│   ├── internal/
│   │   ├── handler/            # HTTP handlers
│   │   │   ├── station.go      # CRUD estações
│   │   │   ├── upload.go       # Upload de músicas
│   │   │   └── websocket.go    # Upgrade WS + eventos
│   │   ├── model/              # Estruturas de dados
│   │   │   ├── station.go      # PlaybackState, Station
│   │   │   └── track.go        # Track (música)
│   │   ├── media/              # Processamento de áudio
│   │   │   └── convert.go      # FFmpeg wrapper
│   │   ├── auth/               # JWT / tokens
│   │   │   └── jwt.go
│   │   └── ws/                 # Gerenciamento WebSocket
│   │       ├── client.go       # Conexão individual
│   │       └── hub.go          # Broadcast por sala
│   ├── go.mod
│   └── go.sum
├── frontend/                   # Vue 3 + Vite
│   ├── src/
│   │   ├── components/         # Componentes Vue
│   │   │   ├── Player.vue
│   │   │   ├── Playlist.vue
│   │   │   ├── Uploader.vue
│   │   │   └── StationList.vue
│   │   ├── composables/        # Lógica reativa
│   │   │   ├── usePlayer.js    # Sync de áudio
│   │   │   └── useWebSocket.js # Conexão WS
│   │   ├── stores/             # Pinia stores
│   │   │   └── station.js
│   │   ├── views/
│   │   │   ├── Home.vue
│   │   │   ├── DJDashboard.vue
│   │   │   └── Listen.vue
│   │   ├── router/
│   │   │   └── index.js
│   │   ├── services/
│   │   │   └── api.js          # Axios instance
│   │   ├── App.vue
│   │   └── main.js
│   ├── index.html
│   ├── vite.config.js
│   ├── tailwind.config.js
│   ├── postcss.config.js
│   └── package.json
└── musicas/                    # Pasta para arquivos .opus
```

## 2. Ordem de Implementação (Faseado)

### FASE 1 — MVP: Conexão e Sincronização Básica

| # | Ação | Arquivos | Detalhes |
|---|------|----------|----------|
| 1.1 | Inicializar módulo Go | `backend/go.mod` | `go mod init radio-backend` |
| 1.2 | Criar `main.go` com servidor HTTP + MIME Opus | `backend/cmd/server/main.go` | Servir `./musicas` em `/musicas/` |
| 1.3 | Adicionar dependências Go | `go get` | gorilla/websocket, go-chi/chi/v5, rs/cors, google/uuid, golang-jwt/jwt/v5, joho/godotenv |
| 1.4 | Criar modelos de dados | `backend/internal/model/station.go`, `track.go` | Structs Station, PlaybackState, Track |
| 1.5 | Criar gerenciamento WebSocket (Hub + Client) | `backend/internal/ws/hub.go`, `client.go` | Hub por stationId, Ping/Pong, broadcast |
| 1.6 | Criar handler WebSocket | `backend/internal/handler/websocket.go` | Upgrade HTTP -> WS, registrar cliente no Hub |
| 1.7 | Criar handler de estações | `backend/internal/handler/station.go` | POST/GET /api/v1/stations |
| 1.8 | Roteador Chi + CORS | `backend/cmd/server/main.go` | Configurar rotas e middleware |
| 1.9 | Inicializar frontend Vite + Vue 3 | `frontend/` | `npm create vite@latest frontend -- --template vue` |
| 1.10 | Instalar deps frontend | `npm install` | vue-router, pinia, axios, tailwindcss, postcss, autoprefixer, lucide-vue-next |
| 1.11 | Configurar Tailwind | `frontend/tailwind.config.js`, `postcss.config.js` | Purge paths, content |
| 1.12 | Criar `useWebSocket` composable | `frontend/src/composables/useWebSocket.js` | Conectar, reconectar, tratar mensagens JSON |
| 1.13 | Criar `usePlayer` composable | `frontend/src/composables/usePlayer.js` | Audio API + sync calc (StartedAt, SeekOffset) |
| 1.14 | Criar Player.vue | `frontend/src/components/Player.vue` | Tag `<audio>` com type opus, controles |
| 1.15 | Criar estação Pinia | `frontend/src/stores/station.js` | Estado global: playlist, playback, role |
| 1.16 | Criar views Home + Listen | `frontend/src/views/Home.vue`, `Listen.vue` | Home: criar/entrar em sala. Listen: player |
| 1.17 | Configurar router | `frontend/src/router/index.js` | `/`, `/radio/:stationId`, `/dj/:stationId` |
| 1.18 | Teste ponta-a-ponta manual | - | Servir .opus estático, ver sync entre 2 abas |

### FASE 2 — Gestão de Salas e Fluxo do DJ

| # | Ação | Arquivos | Detalhes |
|---|------|----------|----------|
| 2.1 | Implementar JWT | `backend/internal/auth/jwt.go` | Gerar/validar djToken |
| 2.2 | Proteger rotas DJ | `backend/cmd/server/main.go` | Middleware de autenticação |
| 2.3 | Implementar upload + conversão | `backend/internal/media/convert.go`, `handler/upload.go` | Receber multipart, chamar FFmpeg, salvar .opus |
| 2.4 | Rota GET /repository | `backend/internal/handler/upload.go` | Listar músicas do repositório |
| 2.5 | Conectar DJ ao WebSocket com token | `backend/internal/handler/websocket.go` | Query param `?role=dj&token=...` |
| 2.6 | Comandos WS: PLAY, PAUSE, SEEK | `backend/internal/ws/hub.go` | Broadcast do PlaybackState atualizado |
| 2.7 | Comandos WS: ADD_TO_PLAYLIST, REMOVE_FROM_PLAYLIST | `backend/internal/ws/hub.go` | Atualizar playlist + broadcast |
| 2.8 | Auto-avanço de faixa | `backend/internal/ws/hub.go` | Goroutine que escuta fim da música (duração) |
| 2.9 | Criar DJDashboard.vue | `frontend/src/views/DJDashboard.vue` | Upload, repositório, playlist, controles |
| 2.10 | Criar Uploader.vue | `frontend/src/components/Uploader.vue` | Drag-and-drop, progresso |
| 2.11 | Criar Playlist.vue | `frontend/src/components/Playlist.vue` | Lista, drag-to-reorder, remover |
| 2.12 | Enviar comandos WS do DJ | `frontend/src/composables/useWebSocket.js` | sendMessage() para play/pause/add/remove |
| 2.13 | Tratar eventos no ouvinte | `frontend/src/composables/usePlayer.js` | Reagir a TRACK_CHANGED, PLAYLIST_UPDATED |
| 2.14 | Tela de autoplay (botão "Entrar") | `frontend/src/components/AutoplayGate.vue` | Botão de interação obrigatória |

### FASE 3 — Otimizações

| # | Ação | Arquivos | Detalhes |
|---|------|----------|----------|
| 3.1 | Implementar janela deslizante (Top 3) | `backend/internal/ws/hub.go` | Payload só envia índices 0,1,2 da playlist |
| 3.2 | Pre-buffer no frontend | `frontend/src/composables/usePlayer.js` | Iniciar download da faixa seguinte |
| 3.3 | Extrair duração com ffprobe | `backend/internal/media/convert.go` | Rodar ffprobe após conversão, armazenar duration |
| 3.4 | Otimizar Ping/Pong | `backend/internal/ws/client.go` | Timeout de leitura, cleanup de conexões mortas |
| 3.5 | Loading states + tratamento de erros | Frontend varios | Toast, spinner, fallback |
| 3.6 | Responsividade / UI refinada | Frontend varios | Tailwind breakpoints, mobile |

## 3. Comandos para Inicialização

```bash
# Backend
cd radio/backend
go mod init radio-backend
go get github.com/gorilla/websocket github.com/go-chi/chi/v5 github.com/rs/cors github.com/google/uuid github.com/golang-jwt/jwt/v5 github.com/joho/godotenv

# Frontend
cd radio
npm create vite@latest frontend -- --template vue
cd frontend
npm install vue-router@4 pinia axios tailwindcss @tailwindcss/vite lucide-vue-next
```

## 4. Protocolo WebSocket (Mensagens JSON)

### Do DJ para o Servidor
```json
{"type": "PLAY"}
{"type": "PAUSE"}
{"type": "SEEK", "position": 45.5}
{"type": "ADD_TO_PLAYLIST", "trackId": "uuid-da-faixa"}
{"type": "REMOVE_FROM_PLAYLIST", "trackId": "uuid-da-faixa"}
{"type": "NEXT_TRACK"}
```

### Do Servidor para Todos (Broadcast)
```json
{"type": "STATE_UPDATE", "state": {"isPlaying": true, "startedAt": 1712345678000, "seekOffset": 0, "currentSong": "/musicas/uuid.opus", "duration": 180.5}}
{"type": "TRACK_CHANGED", "playlist": [track0, track1, track2]}
{"type": "PLAYLIST_UPDATED", "playlist": [track0, track1, track2]}
{"type": "SYNC", "serverTime": 1712345678000}
```

## 5. Decisões Técnicas

- **Armazenamento:** Estado em memória (sem banco de dados) para simplicidade inicial. Arquivos .opus em disco.
- **Portas:** Backend na `:8080`, frontend dev na `:5173` (Vite proxy para backend).
- **FFmpeg:** Necessário instalar no sistema e disponível no PATH.
- **JWT:** Secret em variável de ambiente (`JWT_SECRET`), sem refresh token inicial.
- **Autoplay:** Componente `AutoplayGate.vue` obriga clique do usuário antes de qualquer `audio.play()`.
- **IDs:** UUID v4 para stationId, trackId, djToken.
