# ADR-0003: Migração do monolito para serviços com Strangler Fig

- **Status:** Implementado (Entrega 3)
- **Data:** 2026-11-03 (atualizado em 2026-11-22)
- **Responsável:** Membro 4 — Luiz Paulo (Dados / Station Service)
- **Áreas:** Confiabilidade, Deployment

## Contexto

O RadioSync iniciou como um monolito Go (API REST + WebSocket + transcodificação de mídia + arquivos estáticos). Reescrever tudo do zero (Big-Bang) até a entrega final seria arriscado: a equipe ficaria sem uma versão funcional integrada durante o desenvolvimento.

## Decisão

Evoluir **incrementalmente** aplicando o padrão **Strangler Fig**:

1. Colocar o API Gateway (Traefik v3.6, ADR-0005) na frente do monolito. Todo o tráfego passa pelo gateway — a SPA Vue no cliente não precisa mudar suas URLs de API.
2. Extrair um serviço por vez e alterar apenas as regras de roteamento por `Path` / `PathPrefix` e `priority` no Traefik.
3. O monolito continua respondendo às rotas ainda não migradas (como arquivos estáticos do frontend e painel admin).

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Strangler Fig** (escolhida) | Sempre há uma versão funcional; rollback é trocar uma rota no gateway | Período de convivência com múltiplos containers no Compose |
| Big-bang rewrite | Arquitetura "limpa" desde o início | Alto risco de atraso; nada demonstrável nas etapas intermediárias |
| Manter monolito modular | Menos infraestrutura | Não permite escalar WebSockets, REST e Workers de mídia de forma independente |

## Consequências

- **Positivas:** cada extração foi testada isoladamente e validada mantendo o contrato HTTP/JSON idêntico ao monolito.
- **Negativas / trade-offs:** necessidade de coordenar prioridades de roteamento no Traefik para sub-rotas sob o mesmo prefixo (`/api/v1/stations/*`).

## Implementação (Entrega 3)

### O que já foi migrado

A tabela abaixo consolida o estado atual de todas as rotas no API Gateway Traefik (`docker-compose.yml`):

| Ordem | Rotas no Gateway | Prioridade Traefik | Serviço de Destino | Armazenamento | Status |
|---|---|---|---|---|---|
| 1 | `/ws/stations/*` (`PathPrefix('/ws/')`) | `100` | **Sync Service** (`backend/cmd/sync`, `:8082`, escalável `--scale sync=N`) | Redis Pub/Sub + Estado Quente (ADR-0001) | **Migrado** |
| 2 | `POST /api/v1/stations` e `POST /api/v1/stations/{id}/claim-dj` | `95` | **Station Service** (`backend/cmd/station`, `:8083`, escalável `--scale station=N`) | PostgreSQL 16 + Redis Lock (ADR-0006) | **Migrado** |
| 3 | `GET/PUT /api/v1/stations/*` (estações, playlist, repositório, faixas) e `GET /api/v1/library` | `85` | **Station Service** (`backend/cmd/station`, `:8083`) | PostgreSQL 16 + MinIO (ADR-0006) | **Migrado** |
| 4 | `GET /musicas/*` | `85` | **Station Service** → **MinIO** (`bucket: musicas`) | MinIO S3 ObjectStore (ADR-0006) | **Migrado** |
| 5 | `POST /api/v1/stations/{id}/upload` e `/youtube` | `90` | **Media Service + Worker** (transição via fila RabbitMQ, ADR-0002) | RabbitMQ + MinIO + PostgreSQL | **Em integração (Frente 2)** |
| 6 | `/api/v1/admin/*`, `/api/v1/debug` e `/*` (SPA Frontend) | `100` / `110` / default | **Monolito / Frontend** (`backend/cmd/server`, `:8080`) | PostgreSQL / Estáticos | **Mantido no servidor base** |

### Extração do Station Service (`backend/cmd/station`)

1. **Binário e Container Dedicados:** `backend/cmd/station/main.go` empacotado em `Dockerfile.station` (Alpine enxuto, sem FFmpeg/yt-dlp/Deno), escutando na porta `8083`.
2. **Desacoplamento por Prioridade no Traefik:** Como as rotas de upload (`/api/v1/stations/{id}/upload` e `/youtube`) compartilham o prefixo `/api/v1/stations`, a regra do router `station-api` no Traefik utiliza:
   ```text
   (PathPrefix(`/api/v1/stations`) && !PathRegexp(`^/api/v1/stations/[^/]+/(upload|youtube|cookies.*)$`)) || Path(`/api/v1/library`) || PathPrefix(`/musicas/`)
   ```
   Isso permitiu migrar todas as operações de estação, playlist, biblioteca, `claim-dj` e entrega de áudio para o Station Service mantendo os middlewares de *rate limiting* (`create-station-ratelimit`, `claim-dj-ratelimit`, `api-ratelimit`) sem interferir no roteamento de upload/transcodificação.
3. **Validação de Paridade (`INT-05`):** O teste automatizado `TestINT05_StranglerFigContractParity` (`backend/internal/stationservice/integration_test.go`) executa as mesmas requisições contra os handlers do monolito e contra o Station Service, comprovando paridade exata de status HTTP (`201`, `200`, `401`, `404`), cabeçalhos (`Cache-Control: no-store` na renovação deslizante do `djToken`) e estrutura JSON.
