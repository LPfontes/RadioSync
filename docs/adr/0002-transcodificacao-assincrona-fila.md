# ADR-0002: Transcodificação assíncrona via fila (RabbitMQ) com Retry e DLQ

- **Status:** Implementado (Entrega 3)
- **Data:** 2026-11-03 (atualizado em 2026-11-23)
- **Responsável:** Membro 2 — Davi Alves (Media Service + Worker)
- **Áreas:** Desempenho, Confiabilidade

## Contexto

Hoje o upload (`handler/upload.go`) e o download do YouTube
(`handler/youtube.go`) rodam FFmpeg/yt-dlp **dentro da requisição HTTP**.
Uma conversão leva de segundos a minutos e consome muita CPU. Consequências:

- A requisição fica presa até o fim da conversão (timeout no navegador/proxy).
- Vários uploads simultâneos disputam CPU com o servidor WebSocket, aumentando
  a latência da sincronização para todos os ouvintes.
- Se o yt-dlp falhar (YouTube bloqueando, rede instável), o erro volta direto
  ao usuário; não há nova tentativa.

## Decisão

1. A API passa a apenas **receber o arquivo** (ou a URL do YouTube), gravar o
   original no MinIO e publicar um job na fila `media.transcode` do RabbitMQ.
   Responde `202 Accepted` com um `jobId`.
2. Um **Media Worker** (processo separado, escalável) consome a fila, executa
   FFmpeg/yt-dlp, grava o `.opus` no MinIO e publica `TRACK_READY` (via Redis,
   ADR-0001) para avisar o DJ.
3. **Retry com backoff exponencial** (3 tentativas: 5s, 30s, 2min) e, esgotadas
   as tentativas, a mensagem vai para a **Dead Letter Queue** `media.transcode.dlq`.
4. Chamadas ao YouTube passam por um **Circuit Breaker**: após 5 falhas
   seguidas o circuito abre por 60s e novos jobs de YouTube falham rápido.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Fila RabbitMQ + worker** (escolhida) | DLQ e retry nativos; ack manual; painel de gestão | Mais um componente; consistência eventual (faixa aparece segundos depois) |
| Goroutine no próprio servidor | Simples, sem infra extra | Ainda disputa CPU com o WebSocket; job se perde se o processo cair |
| Redis Streams | Reaproveita o Redis | DLQ/retry precisariam ser implementados à mão |
| Serverless (Lambda/Cloud Run job) | Escala a zero | Não roda localmente com Docker Compose; FFmpeg em ambiente serverless é trabalhoso |

## Consequências

- **Positivas:** API responde rápido; workers escalam de forma independente;
  falhas do YouTube não derrubam nem travam o servidor de sincronização.
- **Negativas / trade-offs:** consistência eventual — o DJ vê a faixa como
  "processando" até o `TRACK_READY`. Mensagens podem ser entregues mais de uma
  vez, então o worker precisa ser **idempotente** (chave = `jobId`).
- **Como vamos validar:** teste de carga com 20 uploads simultâneos medindo
  latência do WebSocket antes/depois; teste de resiliência matando o worker
  no meio de um job (a mensagem deve voltar para a fila); teste forçando falha
  do yt-dlp para verificar retry → DLQ e abertura do circuit breaker.

## Implementação (Entrega 3)

```mermaid
sequenceDiagram
    participant DJ
    participant API as Media API (cmd/media)
    participant S3 as MinIO
    participant MQ as RabbitMQ
    participant W as Media Worker (cmd/worker)
    participant PG as Postgres
    participant S as Sync Service
    DJ->>API: POST /upload ou /youtube
    API->>S3: original em originals/{jobId} (só upload)
    API->>MQ: job em media.transcode (publisher confirm)
    API-->>DJ: 202 + jobId
    MQ->>W: entrega (ack manual)
    W->>W: lock do job no Redis; FFmpeg ou yt-dlp
    W->>S3: {trackId}.opus
    W->>PG: faixa no repositório da estação
    W->>S: TRACK_READY (Redis Pub/Sub)
    S-->>DJ: TRACK_READY (WebSocket)
    W->>MQ: ack
```

| Parte | Onde | Como |
|---|---|---|
| Media API | `internal/mediaservice`, `cmd/media`, `Dockerfile.media` | Valida token, estação, formato e tamanho; responde `202` com `jobId` e `Location`. `GET /api/v1/stations/{id}/jobs` devolve o andamento |
| Fila | `internal/queue` | `media.transcode` (quorum), uma fila de retry por atraso (TTL + dead-letter de volta) e `media.transcode.dlq`. Publicação sempre com *publisher confirm* |
| Worker | `internal/mediaworker`, `cmd/worker`, `Dockerfile.worker` | Ack só depois de o `.opus` e a faixa estarem salvos. Escala com `--scale media-worker=N` |
| Andamento e locks | `internal/jobs` | Redis: registro do job por 24 h e lock com dono e prazo de 15 s, renovado a cada 5 s |
| Circuit breaker | `internal/breaker` | `closed`, `open` e `half-open` com uma chamada de teste por vez |
| Métricas | `internal/metrics/media.go` | `radiosync_media_queue_depth`, `jobs_total`, `job_attempts_total`, `job_duration_seconds`, `jobs_in_progress`, `breaker_state`, `jobs_enqueued_total` |

**Bulkhead.** FFmpeg, yt-dlp e Deno existem só na imagem do worker. Cada réplica tem
limite de CPU e memória no compose e processa no máximo `WORKER_CONCURRENCY` jobs.

**Retry.** A primeira tentativa mais 3 novas, depois de 5 s, 30 s e 2 min
(`MEDIA_RETRY_DELAYS`), e então a DLQ com o motivo no cabeçalho `x-error`. Erros que
não melhoram com o tempo (arquivo corrompido, vídeo privado, estação apagada) vão
direto para a DLQ. Esperar um lock, o Redis fora do ar e o desligamento do worker
não gastam tentativas.

**Mensagem que derruba o worker.** `x-delivery-limit` de 5: depois disso a própria
fila manda a mensagem para a DLQ, em vez de ela derrubar um worker por vez para sempre.

**Idempotência.** A faixa de um upload tem o ID do job, e um job reentregue depois de
pronto só repete o aviso. No YouTube o lock é por vídeo e o `youtube_cache` evita o
segundo download, mesmo com duas estações pedindo ao mesmo tempo. Um worker que perde
o lock cancela o trabalho e não publica.

**Circuit breaker.** Abre após 5 falhas seguidas e testa a volta depois de 60 s. Vídeo
privado ou removido não conta como falha: o YouTube respondeu. Com o circuito aberto
o job falha na hora e gasta uma tentativa; como os atrasos somam 2 min 35 s, mais que
os 60 s do circuito, todo job tem chance de passar por um teste. Uploads não são afetados.

**`TRACK_READY` / `TRACK_FAILED`.** O Pub/Sub não guarda mensagens. Por isso o
andamento também fica no Redis, e o painel consulta `/jobs` a cada 5 s enquanto houver
job ativo. Isso cobre página recarregada e evento perdido numa reconexão.

### Mudanças em relação ao plano

- **Circuit breaker próprio** em vez de `sony/gobreaker`: são ~150 linhas, e precisávamos
  distinguir "o YouTube falhou" de "o worker foi desligado no meio da chamada".
- **Cookies do YouTube** saíram do monolito para a Media API e ficam no volume
  `media-data`, lido pelos workers. Um `cookies.txt` antigo em `./data` precisa ser
  importado de novo pelo painel.
- **Sugestão com link do YouTube:** ao aprovar, o painel do DJ pede o job à Media API. O
  download dentro do monolito foi removido, e a faixa entra no repositório em vez de ir
  direto para a playlist.

### Trade-offs

| Trade-off | Consequência | Como lidamos |
|---|---|---|
| Consistência eventual do repositório | A faixa aparece segundos depois do `202` | Estado "processando…" no painel e consulta a `/jobs` |
| Sem ordem entre mensagens | Um upload curto passa na frente de um longo, e um retry volta para o fim da fila | Aceito: o repositório não depende da ordem |
| Entrega *at-least-once* | O mesmo job pode chegar duas vezes | Worker idempotente |
| Recuperação depende do prazo do lock | Um worker morto segura o job por até 15 s | Prazo menor recuperaria antes, mas uma pausa do processo perderia o trabalho feito |
| Breaker por réplica | Com N workers, o YouTube pode receber até N × 5 falhas antes de todos abrirem | Aceito: não depende de mais um componente e erra para o lado seguro |
| Uma fila para os dois tipos | Downloads lentos ocupam vagas dos uploads | Concorrência e réplicas configuráveis; separar as filas é o próximo passo |
| Originais da DLQ ficam no MinIO | Ocupam espaço até serem apagados à mão | Permitem reprocessar; falta uma regra de expiração |
| O `202` só sai depois do corpo inteiro | Arquivos muito grandes passam de 500 ms pela transferência | O tempo não depende mais da conversão |

### Resultados medidos

Stack real (Docker Compose: Traefik, RabbitMQ, Postgres, Redis, MinIO) numa máquina de
desenvolvimento, com os workers limitados a 1,5 CPU.

| Teste | Cenário | Resultado |
|---|---|---|
| **INT-02** | Upload de 160 KB pelo gateway até `TRACK_READY` no WebSocket | `202` em **181 ms**, faixa pronta 1,0 s depois e áudio tocando |
| **INT-02** (carga) | 20 uploads simultâneos, 2 workers | Todos os `202` entre 112 e **244 ms**; tudo convertido em 3,5 s |
| **INT-02** (arquivo grande) | WAV de 94 MB | `202` entre 1,8 e 6 s na rede interna, quase todo em transferência e disco (13,6 s no pior caso, a partir do Windows). Media API em 13 MB de RAM |
| **RES-02** | `docker kill -s KILL` durante a conversão de um WAV de 94 MB | Outro worker refez o job: **22 s** até `TRACK_READY` (até 15 s do lock e ~8 s de conversão), 1 faixa, nenhuma duplicata. Com o lock de 30 s da primeira versão eram 36 s |
| **RES-03** | yt-dlp que sempre falha, 3 vídeos | 4 tentativas cada, DLQ e `TRACK_FAILED`; circuito aberto na 5ª falha. Atrasos reduzidos para 1, 2 e 4 s |
| YouTube | Vídeo real de 19 s, depois o mesmo vídeo de novo | 32 s na primeira vez e **63 ms** na segunda (cache) |
| Arquivo malicioso | Playlist HLS renomeada para `.mp3`, apontando para um serviço interno | Recusada; o serviço interno não recebeu requisição. O FFmpeg 8 já recusa esse arquivo; o `-protocol_whitelist file` é uma segunda barreira |

Testes automatizados: `internal/mediaworker` (INT-02, RES-02, RES-03, idempotência),
`internal/breaker`, `internal/queue` e `internal/jobs` (RabbitMQ e Redis reais no CI) e
`internal/mediaservice`. O LOAD-03 é do Membro 5.
