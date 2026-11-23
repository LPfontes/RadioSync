# ADR-0006: Persistência poliglota — PostgreSQL, Redis e MinIO

- **Status:** Implementado (Entrega 3)
- **Data:** 2026-11-03 (atualizado em 2026-11-22)
- **Responsável:** Membro 4 — Luiz Paulo (Dados / Station Service)
- **Áreas:** Escalabilidade, Confiabilidade, Desempenho

## Contexto

No monolito original, a persistência era baseada em arquivos locais:

- `stations.json`, `tracks.json`, `track_names.json` e `yt_cache.json` gravados por `handler/persist.go`, com escrita atômica (arquivo temporário + rename) e salvamento periódico a cada 30 segundos.
- Arquivos `.opus` numa pasta local (`MUSIC_DIR`), servidos diretamente pelo processo Go via `http.FileServer`.

Isso funcionava com **uma única** instância. Com múltiplas réplicas do Station Service ou workers de mídia isolados, cada container teria seu próprio `stations.json` e seu próprio diretório local de músicas — causando divergência imediata de estado e arquivos `404 Not Found` entre réplicas.

## Decisão

Cada categoria de dado foi direcionada para o armazenamento mais adequado (**Polyglot Persistence**):

| Dado | Armazenamento | Motivo |
|---|---|---|
| Estado "quente" do player (`PlaybackState`) e lock de DJ | **Redis 7** | Leitura/escrita de altíssima frequência e baixa latência (ADR-0001) |
| Estações, catálogo de faixas, itens da playlist, sugestões e cache YT | **PostgreSQL 16** | Dados duráveis e relacionais; integridade referencial e consultas da biblioteca global |
| Arquivos de áudio `.opus` | **MinIO** (S3-compatível) | Bucket `musicas` compartilhado entre réplicas do Station Service e Media Workers |

Seguindo o padrão **Database-per-service**, o Station Service gerencia o schema relacional no PostgreSQL e expõe as interfaces `storage.Repository` e `storage.ObjectStore` (`backend/internal/storage`) compartilhadas com o Media Worker.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Postgres + Redis + MinIO** (escolhida) | Cada dado no lugar certo; tudo roda no Docker Compose | Três componentes de armazenamento para operar |
| Manter JSON em volume compartilhado | Nenhuma mudança no código | Escrita concorrente de várias réplicas corrompe o arquivo JSON |
| Só Redis (com AOF) | Um componente a menos | Ruim para consultas relacionais e catálogo durável de faixas |
| MongoDB | Esquema flexível | Dados possuem relacionamentos fortes (estação ↔ faixas ↔ playlist); equipe domina SQL |

## Consequências

- **Positivas:** múltiplas réplicas do Station Service e do Media Worker enxergam exatamente as mesmas estações, playlists e arquivos `.opus`; backups e consultas SQL ficam padronizados.
- **Negativas / trade-offs:** consistência eventual entre o estado quente no Redis (`PlaybackState`) e o registro durável no PostgreSQL; necessidade de migração dos dados legados em JSON e `.opus`.

## Implementação (Entrega 3)

A camada de persistência poliglota e o Station Service foram implementados com os seguintes componentes:

1. **Schema Relacional com Migrations Versionadas (`backend/internal/storage/migrations/`):**
   - Embutidas no binário via `embed.FS` e aplicadas automaticamente no boot por `storage.RunMigrations` com controle de versão na tabela `schema_migrations`.
   - Tabelas criadas em `0001_initial_schema.up.sql`:
     - `tracks`: catálogo global de faixas (`id`, `title`, `filename`, `url`, `duration`, `category`, `theme`, `cover_url`).
     - `stations`: registro durável das salas (`id`, `dj_token`, `is_playing`, `started_at`, `seek_offset`, `current_song`, `duration`, `mixer_json`).
     - `station_tracks`: vínculo N:N ordenado do repositório de cada estação (`ON DELETE CASCADE`).
     - `playlist_items`: fila de reprodução ordenada por `position` (`ON DELETE CASCADE`).
     - `suggestions`: sugestões de músicas enviadas pelos ouvintes (`pending`, `approved`, `rejected`).
     - `youtube_cache`: mapeamento `video_id -> track_id` substituindo o antigo `yt_cache.json`.
2. **MinIO `ObjectStore` (`backend/internal/storage/minio.go`) e Áudio sem Disco Local (`backend/internal/handler/audio.go`):**
   - Implementação da interface `ObjectStore` no bucket `musicas` (`EnsureBucket`, `Put`, `Get`, `Exists`, `Delete`, `ListKeys`, `PresignedGetURL`) utilizando protocolo S3 REST (AWS Signature V4) diretamente sobre `net/http` da biblioteca padrão — sem dependências externas vulneráveis ao `govulncheck`.
   - O endpoint `/musicas/{filename}` serve os arquivos `.opus` diretamente do MinIO (suportando tanto streaming HTTP com `Accept-Ranges: bytes` quanto redirecionamento `302 Found` / JSON para URL pré-assinada via `?presign=1` ou `AUDIO_DELIVERY_MODE=presigned`), eliminando 100% a dependência de disco local.
3. **Importador Único e Idempotente (`backend/internal/importer` e `backend/cmd/importer`):**
   - Lê `stations.json`, `track_names.json`, `tracks.json` e `yt_cache.json`, enriquece títulos legados, faz `UPSERT` transacional no PostgreSQL e envia todos os arquivos `.opus` locais para o bucket `musicas` no MinIO.
   - Executado automaticamente na subida do Station Service (ou via CLI `./importer`) de forma totalmente idempotente.
4. **Remoção do `persist.go`:**
   - O arquivo `backend/internal/handler/persist.go` (e a goroutine `PeriodicSave` de escrita JSON a cada 30 s) foi removido do código.

### Trade-offs observados na implementação
- **Proxy Stream vs. URL Pré-assinada no ambiente Docker local:** Como o MinIO roda na rede interna do Compose (`http://minio:9000`) e o navegador do usuário resolve `http://localhost`, o modo padrão `proxy` transmite o stream do MinIO pelo próprio Station Service em `/musicas/{filename}` sem exigir que a porta `9000` esteja aberta publicamente, mantendo o suporte a URLs pré-assinadas (`PresignedGetURL`) para acesso direto ou CDN (`S3_PUBLIC_URL`).
- **Degradação graciosa quando o Postgres cai (`RES-05`):** Como o `PlaybackState` quente e o fan-out residem no Redis (ADR-0001), a queda do PostgreSQL não interrompe nenhuma transmissão em andamento; apenas operações de escrita/criação de estação retornam `503 Service Unavailable` com erro JSON explícito.

### Resultados Medidos nos Testes Automatizados

| Teste | Cenário | Critério de Sucesso | Resultado |
|---|---|---|---|
| **INT-04** | Estação e playlist criadas na Réplica A do Station Service são lidas na Réplica B | Mesmos dados (`id`, `playlist`, `repository`, `djToken` e áudio `.opus`) | **Aprovado** (`TestINT04_ReplicaStationConsistency`) |
| **RES-05** | Postgres indisponível durante transmissão ativa | Ouvintes continuam recebendo `PLAY`/`STATE_UPDATE` via Redis; `POST /api/v1/stations` retorna HTTP `503` com erro claro | **Aprovado** (`TestRES05_PostgresDown_ListenersContinueAndCreateStationFailsClearly`) |
| **Repositório & Migrations** | Execução das migrations SQL, CRUD de estações, ordenação de playlist e sugestões | 100% de consistência relacional | **Aprovado** (`TestPostgresRepository_Integration`) |
| **Importador Único** | Migração de `stations.json`, `track_names.json`, `yt_cache.json` e `.opus` executada 2x seguidas | Dados migrados e 2ª execução idempotente (0 duplicatas / 0 re-uploads) | **Aprovado** (`TestImporter_JSONAndOpus_Idempotent`) |
