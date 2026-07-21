# 📻 RadioSync — Rádio Web Sincronizada em Tempo Real

**RadioSync** é uma aplicação web moderna para transmissão e escuta sincronizada de rádio online em tempo real. Permite que um **DJ** crie uma estação, faça upload de músicas (convertidas automaticamente para Opus), gerencie a fila de reprodução e sincronize os controles de reprodução (Play, Pause, Seek, Troca de Faixa) com múltiplos **Ouvintes** conectados via WebSockets.

---

## 🚀 Tecnologias Utilizadas

### **Backend**
- **Linguagem**: [Go (Golang)](https://go.dev/) (v1.22+)
- **Router HTTP**: [chi v5](https://github.com/go-chi/chi)
- **WebSockets**: [gorilla/websocket](https://github.com/gorilla/websocket)
- **Autenticação**: [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt)
- **Processamento de Áudio**: [FFmpeg](https://ffmpeg.org/) (transcodificação para codec `.opus` a 96kbps VBR)
- **Gerenciador de Ambientes**: [godotenv](https://github.com/joho/godotenv)

### **Frontend**
- **Framework**: [Vue.js 3](https://vuejs.org/) (Composition API com `<script setup>`)
- **Build Tool**: [Vite](https://vitejs.dev/)
- **Gerenciamento de Estado**: [Pinia](https://pinia.vuejs.org/)
- **Roteamento**: [Vue Router 4](https://router.vuejs.org/)
- **Estilização**: [Tailwind CSS](https://tailwindcss.com/)
- **Ícones**: [Lucide Vue Next](https://lucide.dev/)

### **Infraestrutura**
- **Containerização**: Docker (Multi-stage build servindo a SPA Vue e a API Go em uma única imagem otimizada Alpine).

---

## ✨ Funcionalidades Principais

- 🎙️ **Criação de Estações de Rádio (DJ)**:
  - Geração de código seguro de 6 caracteres (ex: `X7K2P9`) via `crypto/rand`.
  - Emissão de Token JWT assinado para permissões administrativas do DJ.
- 🎵 **Upload e Transcodificação Automática**:
  - Aceita múltiplos formatos de áudio (`.mp3`, `.wav`, `.ogg`, `.flac`, `.aac`, `.m4a`).
  - Conversão em tempo real no servidor para **Opus (`.opus`)**, reduzindo o consumo de banda e otimizando o stream de áudio.
- 🔄 **Sincronização em Tempo Real (WebSockets)**:
  - Eventos de `PLAY`, `PAUSE`, `SEEK` e `NEXT_TRACK` retransmitidos instantaneamente para todos os ouvintes.
  - Sincronização automática do tempo decorrido ao entrar em uma sala em andamento.
- 📑 **Gerenciamento de Repositório e Fila (Playlist)**:
  - O DJ pode adicionar músicas do repositório local ou da **Biblioteca Global** do servidor à fila, remover itens ou pular faixas.
- 🌐 **Biblioteca Global de Músicas**:
  - Exibe todas as faixas enviadas para o servidor (independentemente de qual estação realizou o upload) com busca por título e sincronização instantânea.
- 💾 **Persistência Atômica de Dados**:
  - Salvamento periódico e por eventos das estações no arquivo `stations.json` utilizando escrita temporária e substituição atômica (*atomic rename*).
- 🕒 **Histórico Local no Navegador**:
  - Armazenamento das últimas salas visitadas no LocalStorage para acesso rápido.

---

## 🛠️ Como Executar o Projeto

### Pró-requisitos
- **Go** >= 1.22
- **Node.js** >= 18 e **npm**
- **FFmpeg** e **FFprobe** instalados e disponíveis no `PATH` do sistema.

---

### 1. Executando Localmente (Desenvolvimento)

#### **Backend**
```bash
cd backend
go run ./cmd/server/main.go
```
*O backend estará rodando em `http://localhost:8080`.*

#### **Frontend**
```bash
cd frontend
npm install
npm run dev
```
*O frontend estará rodando em `http://localhost:5173`.*

---

### 2. Executando via Docker (Produção / Produção Local)

O projeto possui um [Dockerfile](file:///c:/Users/lpfon/Downloads/radio/Dockerfile) multi-stage que compila o frontend, o backend e instala as dependências do FFmpeg na imagem final.

#### **Construir a imagem Docker:**
```bash
docker build -t radiosync .
```

#### **Executar o container:**
```bash
docker run -d -p 8080:8080 --name radiosync radiosync
```

Acesse a aplicação no navegador em `http://localhost:8080`.

---

## ⚙️ Variáveis de Ambiente

As principais variáveis configuráveis no servidor Go:

| Variável | Valor Padrão | Descrição |
| :--- | :--- | :--- |
| `PORT` | `8080` | Porta onde o servidor HTTP/WS irá escutar. |
| `MUSIC_DIR` | `../musicas` (ou `/app/musicas`) | Diretório onde os arquivos `.opus` convertidos são armazenados. |
| `DATA_DIR` | `./data` (ou `/app/data`) | Diretório onde o arquivo de estado `stations.json` é armazenado. |
| `FRONTEND_DIR` | `../frontend/dist` (ou `/app/frontend/dist`) | Diretório contendo os arquivos estáticos compilados da SPA Vue. |
| `JWT_SECRET` | `dev-secret-change-in-production` | Chave secreta usada para assinar e validar tokens JWT do DJ. |

---

## 📡 Endpoints da API REST e WebSockets

### **API REST (`/api/v1`)**

- `POST /api/v1/stations` — Cria uma nova estação e retorna `{ stationId, djToken }`.
- `GET /api/v1/stations/{stationId}` — Retorna o estado atual, a playlist e o repositório da estação.
- `POST /api/v1/stations/{stationId}/upload` — Realiza upload de arquivo de áudio (Requer header `Authorization: Bearer <djToken>`).
- `GET /api/v1/stations/{stationId}/repository` — Lista as faixas do repositório da estação (Requer header `Authorization: Bearer <djToken>`).
- `GET /api/v1/stations/{stationId}/musicas` — Lista os arquivos `.opus` existentes no servidor.
- `GET /api/v1/library` — Lista todas as músicas da biblioteca global salvas no servidor.
- `GET /api/v1/debug` — Endpoint de diagnóstico de status do servidor e persistência.

### **WebSockets (`/ws`)**

- `WS /ws/stations/{stationId}?role=dj&token=<djToken>` — Conexão WebSocket para a estação.

#### **Mensagens Enviadas pelo DJ (`IncomingMessage`):**
- `{ "type": "PLAY" }` — Inicia a reprodução.
- `{ "type": "PAUSE" }` — Pausa a reprodução.
- `{ "type": "SEEK", "data": { "position": 45.5 } }` — Altera o tempo da música.
- `{ "type": "NEXT_TRACK" }` — Pula para a próxima faixa da playlist.
- `{ "type": "ADD_TO_PLAYLIST", "data": { "trackId": "..." } }` — Adiciona música à fila.
- `{ "type": "REMOVE_FROM_PLAYLIST", "data": { "trackId": "..." } }` — Remove música da fila.
- `{ "type": "SYNC_REQUEST" }` — Solicita sincronização imediata de tempo.

#### **Mensagens Transmitidas pelo Servidor (`OutgoingMessage`):**
- `{ "type": "STATE_UPDATE", "state": { ... } }` — Atualizações de estado do player (reproduzindo, música atual, tempo decorrido).
- `{ "type": "PLAYLIST_UPDATED", "playlist": [ ... ] }` — Fila de reprodução atualizada.
- `{ "type": "SYNC", "position": 12.4 }` — Resposta de sincronização precisa de tempo.

---

## 📂 Estrutura de Diretórios

```
radio/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go                 # Ponto de entrada do servidor HTTP/WS
│   ├── internal/
│   │   ├── auth/                       # Geração e validação de tokens JWT
│   │   ├── handler/                    # Handlers REST, WebSocket e Persistência
│   │   ├── media/                      # Transcodificação e leitura de duração via FFmpeg
│   │   ├── model/                      # Modelos de Estação, Faixas e Playback
│   │   └── ws/                         # Hub WebSocket e gerenciamento de Clientes
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── assets/                     # Estilos globais
│   │   ├── components/                 # Componente Player de Áudio
│   │   ├── composables/                # Hooks reativos (usePlayer, useWebSocket)
│   │   ├── router/                     # Configuração de rotas (Home, DJDashboard, Listen)
│   │   ├── services/                   # Chamadas Axios e LocalStorage
│   │   ├── stores/                     # Estado global da Estação (Pinia)
│   │   ├── views/                      # Páginas da aplicação
│   │   ├── App.vue
│   │   └── main.js
│   ├── package.json
│   └── vite.config.js
├── musicas/                            # Armazenamento de arquivos de áudio em Opus
├── Dockerfile                          # Build multi-stage para deploy unificado
├── Plano de Arquitetura.md
└── README.md
```

---

## 📄 Licença

Este projeto é de código aberto e está disponível sob a licença MIT.
