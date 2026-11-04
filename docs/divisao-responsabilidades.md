# Divisão de responsabilidades

> Preencham os nomes e usuários do GitHub. A nota individual considera os
> commits, então cada pessoa deve commitar a própria frente com a própria conta.

| Membro | Nome / GitHub | Frente | Entrega 2 (06/11) | Entrega 3 (11/12) |
|---|---|---|---|---|
| 1 | _preencher_ | **Sync Service** (WebSocket) | C4 nível 2, ADR-0001, ADR-0004 | Fan-out via Redis Pub/Sub, estado no Redis, réplicas, correção de offset de relógio |
| 2 | _preencher_ | **Media Service + Worker** | ADR-0002 | Fila RabbitMQ, worker idempotente, retry/DLQ, circuit breaker no yt-dlp |
| 3 | _preencher_ | **Infra / DevOps / Segurança** | docker-compose, CI, ADR-0005, remoção de segredos | Auth admin, CORS, regras de rate limit, pipeline com scan de segurança, estratégia de deployment |
| 4 | _preencher_ | **Dados / Station Service** | ADR-0003, ADR-0006, stack.md | Postgres + MinIO, migração do `stations.json`, extração do Station Service |
| 5 | _preencher_ | **Testes + Observabilidade** | C4 nível 1, plano de testes, padrões | Endpoint `/metrics`, painéis Grafana, scripts k6, testes de resiliência e relatório de resultados |

## Responsabilidades transversais

- **Revisão de PR:** todo PR precisa de 1 aprovação de outro membro.
- **README e seção "Ferramentas de IA utilizadas":** cada membro escreve a
  parte da própria frente.
- **Apresentação:** cada membro apresenta a própria frente (~3 min).
