# Testes de resiliência (falha real, com `docker stop`)

Complementam os testes RES em Go (`tests/integration/README.md`), que usam
fakes em memória. Aqui a falha é de verdade: o script derruba um container do
Compose no meio de uma carga e confere se o sistema se recupera.

```bash
docker compose up -d --build        # a stack precisa estar no ar
tests/resilience/res-01-replica-sync.sh
tests/resilience/res-04-redis.sh
tests/resilience/res-05-postgres.sh
```

Cada script sobe a sonda `probe-ouvintes.js` (k6, dentro da rede do Compose):
ouvintes WebSocket que reconectam sozinhos, como o frontend, e medem quanto
levaram para voltar a receber o estado. As metas viram *thresholds* do k6 e o
script sai com código ≠ 0 se falharem.

| ID | Falha injetada | Meta |
|---|---|---|
| RES-01 | `docker stop` de uma das 2 réplicas do sync | p95 da reconexão < 5 s; nenhum ouvinte fica sem voltar |
| RES-04 | `docker compose restart redis` com DJ tocando | nenhum ouvinte fica > 5 s sem evento; dados preservados (AOF) |
| RES-05 | `docker compose stop postgres` | ouvintes seguem escutando; `POST /stations` falha em ≤ 10 s com 5xx |

**RES-02 e RES-03** (worker morto no meio da conversão, `yt-dlp` falhando) já
rodam com RabbitMQ real nos testes Go de `internal/mediaworker`; não há versão
com `docker stop` por enquanto.

## Estado da validação

| Script | Validado? |
|---|---|
| RES-01 | **Sim**, numa mini-stack isolada (2 réplicas do sync atrás do Traefik, Redis): 15 de 30 ouvintes caíram, todos reconectaram, p95 = 2,39 s |
| RES-04 | **Sim**, mesma mini-stack: Redis reiniciado com DJ ativo, 0 ouvintes perdidos, último evento há 1,95 s, AOF manteve as chaves |
| RES-05 | **Não.** Só a sintaxe. Precisa da stack completa (station service, Postgres, MinIO) |

Na mini-stack a falha foi injetada com `docker stop`/`docker restart` direto,
porque ela não tem o `docker compose`. Os scripts `.sh` em si **ainda não foram
executados** contra o Compose completo.

## Observações

- Pela leitura do código (não testado): se a réplica do DJ cair, o lock de DJ
  no Redis só expira após 15 s, e uma nova conexão que chega antes disso entra
  como ouvinte. Por isso o RES-01 roda sem DJ. Vale um teste dedicado.
- `COMPOSE_NETWORK=<rede>` se o projeto não se chama como a pasta.
