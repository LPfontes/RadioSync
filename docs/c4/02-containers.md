# C4 — Nível 2: Diagrama de Containers

Dois diagramas: como o sistema **é hoje** (monolito) e a **arquitetura-alvo** para a Entrega 3. A passagem de um para o outro segue o Strangler Fig (ADR-0003).

## 2.1 Situação atual (as-is)

```mermaid
C4Container
    title RadioSync — Containers (situação atual)

    Person(dj, "DJ")
    Person(ouvinte, "Ouvinte")
    System_Ext(youtube, "YouTube")

    System_Boundary(rs, "RadioSync") {
        Container(spa, "Frontend SPA", "Vue 3, Vite, Pinia", "Player, painel do DJ, tela do ouvinte.")
        Container(mono, "Servidor RadioSync", "Go, chi, gorilla/websocket", "API REST, Hub WebSocket por estação, upload + FFmpeg, yt-dlp, arquivos estáticos.")
        ContainerDb(json, "stations.json / catálogos", "Arquivos JSON em disco", "Estado persistido com escrita atômica.")
        ContainerDb(disk, "Pasta musicas/", "Disco local", "Arquivos .opus.")
    }

    Rel(dj, spa, "Usa", "HTTPS")
    Rel(ouvinte, spa, "Usa", "HTTPS")
    Rel(spa, mono, "REST + WebSocket", "JSON")
    Rel(mono, json, "Lê/grava")
    Rel(mono, disk, "Lê/grava .opus")
    Rel(mono, youtube, "yt-dlp", "HTTPS")
```

**Limitações que motivam a evolução:** estado em memória e em arquivo local (não dá para ter duas réplicas); transcodificação bloqueia a requisição e disputa CPU com o WebSocket; rotas admin sem autenticação.

## 2.2 Arquitetura-alvo (to-be)

```mermaid
C4Container
    title RadioSync — Containers (arquitetura-alvo)

    Person(dj, "DJ")
    Person(ouvinte, "Ouvinte")
    System_Ext(youtube, "YouTube")

    System_Boundary(rs, "RadioSync") {
        Container(spa, "Frontend SPA", "Vue 3", "Servida como arquivos estáticos.")
        Container(gw, "API Gateway", "Traefik", "Roteamento, load balancing, rate limit, auth de borda.")
        Container(station, "Station Service", "Go", "Estações, playlist, biblioteca, tokens de DJ.")
        Container(sync, "Sync Service (N réplicas)", "Go, WebSocket", "Recebe comandos do DJ e faz fan-out para ouvintes locais.")
        Container(media, "Media API", "Go", "Recebe upload/URL e enfileira jobs.")
        Container(worker, "Media Worker (N réplicas)", "Go, FFmpeg, yt-dlp", "Transcodifica para Opus; retry, DLQ e circuit breaker.")
        ContainerDb(redis, "Redis", "Redis 7", "Estado quente do player + Pub/Sub de eventos.")
        ContainerQueue(mq, "RabbitMQ", "AMQP", "Fila media.transcode + DLQ.")
        ContainerDb(pg, "PostgreSQL", "Postgres 16", "Estações e catálogo de faixas.")
        ContainerDb(s3, "MinIO", "S3", "Arquivos .opus e originais.")
        Container(obs, "Prometheus + Grafana", "Observabilidade", "Métricas e painéis.")
    }

    Rel(dj, spa, "Usa")
    Rel(ouvinte, spa, "Usa")
    Rel(spa, gw, "REST + WebSocket", "HTTPS/WSS")
    Rel(spa, s3, "Baixa .opus", "URL pré-assinada")
    Rel(gw, station, "/api/v1/stations")
    Rel(gw, sync, "/ws/stations")
    Rel(gw, media, "/upload, /youtube")
    Rel(station, pg, "Lê/grava", "SQL")
    Rel(station, redis, "Estado da estação")
    Rel(sync, redis, "Pub/Sub + estado")
    Rel(media, s3, "Grava original")
    Rel(media, mq, "Publica job")
    Rel(worker, mq, "Consome job")
    Rel(worker, s3, "Grava .opus")
    Rel(worker, redis, "Publica TRACK_READY")
    Rel(worker, youtube, "yt-dlp")
    Rel(obs, gw, "Coleta métricas")

    UpdateLayoutConfig($c4ShapeInRow="4", $c4BoundaryInRow="1")
```

## Containers da arquitetura-alvo

| Container | Tecnologia | Responsabilidade | ADR |
|---|---|---|---|
| API Gateway | Traefik | Entrada única, LB, rate limit, auth de borda, roteamento do Strangler Fig | 0003, 0005 |
| Station Service | Go | CRUD de estações, playlist, biblioteca, emissão do `djToken` | 0006 |
| Sync Service | Go + WebSocket | Hub por estação; fan-out entre réplicas via Redis | 0001, 0004 |
| Media API | Go | Aceita upload/URL, responde `202 Accepted`, publica job | 0002 |
| Media Worker | Go + FFmpeg + yt-dlp | Transcodificação idempotente; retry, DLQ, circuit breaker | 0002 |
| Redis | Redis 7 | Estado quente e Pub/Sub | 0001 |
| RabbitMQ | RabbitMQ 3.13 | Fila de jobs + DLQ | 0002 |
| PostgreSQL | Postgres 16 | Dados duráveis | 0006 |
| MinIO | S3 compatível | Arquivos de áudio | 0006 |
| Prometheus / Grafana | — | Métricas (latência p95, filas, conexões) | — |

## 2.3 Situação Implementada — Entrega 3 (as-built / Strangler Fig)

Na Entrega 3, o **Sync Service**, o **Station Service** e o **Media Service + Worker** foram extraídos do monolito segundo o padrão **Strangler Fig** (ADR-0003). O Traefik intercepta todo tráfego de `/ws/*` com prioridade 100 e faz balanceamento de carga entre as réplicas ativas do Sync Service (`docker compose up -d --scale sync=N`).

```mermaid
C4Container
    title RadioSync — Containers Implementados (Entrega 3)

    Person(dj, "DJ")
    Person(ouvinte, "Ouvinte")
    System_Ext(youtube, "YouTube")

    System_Boundary(rs, "RadioSync") {
        Container(spa, "Frontend SPA", "Vue 3, Pinia", "Player, painel do DJ, tela de escuta e cálculo de offset NTP.")
        Container(gw, "API Gateway / LB", "Traefik v3.6", "Roteia cada rota para o seu serviço, com rate limit.")
        Container(sync, "Sync Service (N réplicas)", "Go (cmd/sync), WebSocket", "Hub por estação, fan-out Redis Pub/Sub, estado quente, heartbeat e lock de DJ.")
        Container(mono, "Servidor RadioSync (Monolito)", "Go (cmd/server)", "Rotas de admin e SPA. Sem FFmpeg nem yt-dlp.")
        Container(station, "Station Service (N réplicas)", "Go (cmd/station)", "Estações, playlist, biblioteca, tokens de DJ e áudio do MinIO.")
        Container(media, "Media API (N réplicas)", "Go (cmd/media)", "Valida upload/URL, guarda o original, enfileira e responde 202.")
        Container(worker, "Media Worker (N réplicas)", "Go (cmd/worker), FFmpeg, yt-dlp", "Converte com ack manual, retry, DLQ e circuit breaker.")
        ContainerDb(redis, "Redis", "Redis 7 (AOF)", "Canal station:{id}:events, estado, lock de DJ, andamento e lock dos jobs de mídia.")
        ContainerQueue(mq, "RabbitMQ", "RabbitMQ 3.13", "media.transcode, filas de retry e media.transcode.dlq.")
        ContainerDb(pg, "PostgreSQL", "Postgres 16", "Banco de dados relacional durável.")
        ContainerDb(minio, "MinIO", "S3 Storage", "Armazenamento de objetos de mídia.")
        Container(obs, "Prometheus + Grafana", "Observabilidade", "Scrape de /metrics do gateway e das réplicas do Sync Service.")
    }

    Rel(dj, spa, "Usa", "HTTPS")
    Rel(ouvinte, spa, "Usa", "HTTPS")
    Rel(spa, gw, "Requisições HTTP e conexões WS", "HTTPS/WSS")

    Rel(gw, sync, "Roteia /ws/stations/* com load balancing", "WebSocket")
    Rel(gw, mono, "Roteia admin e estáticos", "HTTP")
    Rel(gw, station, "Roteia /api/v1/stations* e /musicas", "HTTP")
    Rel(gw, media, "Roteia /upload, /youtube, /jobs e /cookies", "HTTP")

    Rel(sync, redis, "Pub/Sub (events), estado quente (state) e lock DJ", "TCP/RESP")
    Rel(station, pg, "Lê/grava", "SQL")
    Rel(station, minio, "Lê .opus", "S3")
    Rel(media, minio, "Grava original", "S3")
    Rel(media, mq, "Publica job", "AMQP")
    Rel(worker, mq, "Consome job", "AMQP")
    Rel(worker, minio, "Grava .opus", "S3")
    Rel(worker, pg, "Grava faixa", "SQL")
    Rel(worker, redis, "TRACK_READY / TRACK_FAILED", "TCP/RESP")
    Rel(worker, youtube, "yt-dlp", "HTTPS")

    Rel(obs, sync, "Raspa /metrics (conexões ativas, latência fan-out)", "HTTP")
    Rel(obs, media, "Raspa /metrics (profundidade das filas)", "HTTP")
    Rel(obs, worker, "Raspa /metrics (jobs, tentativas, breaker)", "HTTP")
    Rel(obs, gw, "Raspa métricas de requisições", "HTTP")

    UpdateLayoutConfig($c4ShapeInRow="4", $c4BoundaryInRow="1")
```

> Os diagramas usam a sintaxe C4 do Mermaid, renderizada pelo GitHub. Se precisarem de imagem para os slides, abram o arquivo no [Mermaid Live Editor](https://mermaid.live) e exportem em PNG/SVG.
