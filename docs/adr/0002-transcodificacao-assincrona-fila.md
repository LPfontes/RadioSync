# ADR-0002: Transcodificação assíncrona via fila (RabbitMQ) com Retry e DLQ

- **Status:** Aceito (implementação na Entrega 3)
- **Data:** 2026-11-03
- **Responsável:** Membro 2 (Media Service)
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
