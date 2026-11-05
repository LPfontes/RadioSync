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

> Os diagramas usam a sintaxe C4 do Mermaid, renderizada pelo GitHub. Se precisarem de imagem para os slides, abram o arquivo no [Mermaid Live Editor](https://mermaid.live) e exportem em PNG/SVG.
