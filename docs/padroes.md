# Padrões arquiteturais aplicados

O documento da disciplina exige no mínimo 3 padrões da Seção 5 e ao menos
2 áreas técnicas. O RadioSync cobre **4 áreas** com os padrões abaixo.

| # | Padrão (Seção 5) | Área | Onde se aplica no RadioSync | Situação | ADR |
|---|---|---|---|---|---|
| 1 | **Queues, PubSub e Messaging Fanout** | Escalabilidade | Redis Pub/Sub repassa eventos do DJ a todas as réplicas do Sync Service | Entrega 3 | 0001 |
| 2 | **Stateless, Stateful e Imutabilidade** | Escalabilidade | Estado da estação sai da memória do processo e vai para Redis | Entrega 3 | 0001 |
| 3 | **Load Balancing** | Escalabilidade | Traefik distribui conexões entre réplicas | Compose pronto | 0005 |
| 4 | **Retry Pattern e DLQ** | Confiabilidade | Jobs de transcodificação: 3 tentativas com backoff → `media.transcode.dlq` | Entrega 3 | 0002 |
| 5 | **Circuit Breaker** | Confiabilidade | Chamadas ao YouTube (yt-dlp) | Entrega 3 | 0002 |
| 6 | **Strangler Fig** | Confiabilidade | Extração gradual dos serviços do monolito via gateway | Em andamento | 0003 |
| 7 | **SYNC vs ASYNC / Arquitetura orientada a eventos** | Desempenho | Upload responde `202` e a conversão ocorre de forma assíncrona; `TRACK_READY` como evento | Entrega 3 | 0002 |
| 8 | **Polyglot Persistence / Database-per-service** | Desempenho | Redis (estado quente), Postgres (durável), MinIO (arquivos) | Entrega 3 | 0006 |
| 9 | **API Gateway Pattern** (auth, rate limiting) | Segurança | Traefik na borda; rate limit já configurado | Compose pronto | 0005 |
| 10 | **Rate Limit / Throttling** | Desempenho / Segurança | Limite por IP no gateway, mais rígido para upload e criação de salas | Parcial | 0005 |

**Núcleo obrigatório (os 3 que vamos demonstrar com testes):**
Messaging Fanout (1), Retry + DLQ (4) e Circuit Breaker (5).
Os demais aprofundam a solução e contam para o critério de Originalidade.

## Já existentes no código

- **Janela deslizante / pre-buffer**: só as 3 próximas faixas são enviadas ao ouvinte.
- **Escrita atômica** (temp + rename) do `stations.json` — será substituída pelo Postgres.
- **Ping/Pong com deadlines** no WebSocket para detectar conexões mortas.
