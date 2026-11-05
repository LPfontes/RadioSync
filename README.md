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
- 🔴 **Download de Músicas do YouTube (yt-dlp)**:
  - Permite baixar o áudio de qualquer vídeo do YouTube colando o link no painel do DJ. O servidor usa o **yt-dlp** para extrair a melhor qualidade de áudio e converter diretamente em Opus.
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
- **FFmpeg**, **FFprobe** e **yt-dlp** instalados e disponíveis no `PATH` do sistema.

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

O projeto possui um [Dockerfile](Dockerfile) multi-stage que compila o frontend, o backend e instala as dependências do FFmpeg e yt-dlp na imagem final.

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

### 3. Ambiente completo com Docker Compose (recomendado)

Sobe a aplicação junto com a infraestrutura da arquitetura distribuída
(API Gateway, Redis, RabbitMQ, PostgreSQL, MinIO, Prometheus e Grafana):

```bash
cp .env.example .env        # e troque o JWT_SECRET
docker compose up -d --build
```

| Serviço | Endereço |
|---|---|
| Aplicação (via API Gateway) | http://localhost |
| Aplicação (direto) | http://localhost:8080 |
| Traefik dashboard | http://localhost:8081 |
| RabbitMQ management | http://localhost:15672 |
| MinIO console | http://localhost:9001 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |

---

### 4. Testes

```bash
cd backend
go test -race ./...
```

O CI (GitHub Actions, `.github/workflows/ci.yml`) roda `go vet`, os testes com
detector de race condition, o build do frontend, a validação do
`docker-compose.yml` e o build da imagem Docker em todo push e pull request.

---

## 🏛️ Documentação de Arquitetura

| Documento | Conteúdo |
|---|---|
| [C4 — Nível 1: Contexto](docs/c4/01-contexto.md) | Atores e sistemas externos |
| [C4 — Nível 2: Containers](docs/c4/02-containers.md) | Situação atual e arquitetura-alvo |
| [ADRs](docs/adr/) | Registros de decisões arquiteturais (0001–0006) |
| [Stack tecnológico](docs/stack.md) | Tecnologias e justificativas |
| [Padrões arquiteturais](docs/padroes.md) | Padrões da disciplina aplicados ao RadioSync |
| [Plano de testes](docs/plano-de-testes.md) | Unitários, integração, carga e resiliência |
| [Divisão de responsabilidades](docs/divisao-responsabilidades.md) | Frentes de cada integrante |

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
- `POST /api/v1/stations/{stationId}/youtube` — Baixa e converte o áudio de um link do YouTube via `yt-dlp` (Requer header `Authorization: Bearer <djToken>`).
- `GET /api/v1/stations/{stationId}/repository` — Lista as faixas do repositório da estação (Requer header `Authorization: Bearer <djToken>`).
- `GET /api/v1/stations/{stationId}/musicas` — Lista os arquivos `.opus` existentes no servidor.
- `GET /api/v1/library` — Lista todas as músicas da biblioteca global salvas no servidor.
- `GET /api/v1/debug` — Endpoint de diagnóstico de status do servidor e persistência.

### **Administração (`/api/v1/admin`)**

- `GET /api/v1/admin/stations` — Retorna resumo completo de todas as estações e métricas do `stations.json`.
- `DELETE /api/v1/admin/stations/{stationId}` — Exclui uma estação permanentemente e salva o `stations.json`.
- `POST /api/v1/admin/purge-orphans` — Purga todos os metadados de faixas inexistentes do `stations.json`.
- `DELETE /api/v1/admin/stations/{stationId}/tracks/{trackId}` — Remove uma faixa do repositório da estação.

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

## 🤖 Ferramentas de IA utilizadas

> Seção obrigatória pelo documento da disciplina — a omissão desclassifica a
> entrega. **Cada integrante deve revisar e completar a parte da sua frente.**

| Ferramenta | Onde atuou | Como foi orientada | Avaliação honesta |
|---|---|---|---|
| Claude (Anthropic) | Entrega 2: rascunho dos ADRs 0001–0006, diagramas C4 em Mermaid, plano de testes, `docker-compose.yml` com a infraestrutura, workflow de CI e testes unitários Go | Recebeu o documento do projeto final e o link do repositório; leu o código atual e gerou os artefatos a partir dele | Os testes encontraram um bug real em `ExtractYouTubeURL` (corrigido). A equipe revisou os ADRs e ajustou: _descrever aqui o que foi alterado ou descartado_ |
| Claude (Anthropic), via Claude Code — João Vitor Sampaio Costa (Membro 5) | Frente de Testes + Observabilidade: C4 nível 1 (`docs/c4/01-contexto.md`), template de ADR, mapeamento de padrões arquiteturais (`docs/padroes.md`), plano de testes preliminar e configuração inicial de Prometheus e Grafana (`infra/`). Também auxiliou na aplicação dos patches, nos commits/PRs e em subir o Docker Compose para teste | Recebeu o enunciado da tarefa e o repositório; leu o código atual e gerou os documentos a partir dele | O conteúdo foi gerado a partir do código existente, não de decisões prévias do grupo. Validei a configuração subindo a stack da `dev`: os alvos `prometheus` e `traefik` ficam UP e o `radiosync` fica DOWN até o endpoint `/metrics` ser implementado (Entrega 3) |
| _ferramenta usada no desenvolvimento inicial (ex.: Gemini, Copilot)_ | _ex.: geração do Plano de Arquitetura e de partes do backend/frontend_ | _preencher_ | _preencher_ |
| _outras_ | | | |

---

## 📄 Licença

Este projeto é de código aberto e está disponível sob a licença MIT.
