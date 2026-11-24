# Testes de resiliência (falha real, com `docker stop`)

Complementam os testes RES em Go (`tests/integration/README.md`), que usam
fakes em memória. Aqui a falha é de verdade: o script derruba um container do
Compose no meio de uma carga e confere se o sistema se recupera.

```bash
docker compose up -d --build        # a stack precisa estar no ar
tests/resilience/res-01-replica-sync.sh
tests/resilience/res-02-worker-morto.sh
tests/resilience/res-03-ytdlp.sh
tests/resilience/res-04-redis.sh
tests/resilience/res-05-postgres.sh
```

Se já houver outra stack rodando na máquina, use a stack isolada
(`tests/stack-isolada/README.md`) e aponte os scripts para ela.

RES-01, 04 e 05 sobem a sonda `probe-ouvintes.js` (k6, dentro da rede do
Compose): ouvintes WebSocket que reconectam sozinhos, como o frontend, e medem
quanto levaram para voltar a receber o estado. As metas viram *thresholds* do k6
e o script sai com código ≠ 0 se falharem.

| ID | Falha injetada | Meta |
|---|---|---|
| RES-01 | `docker stop` de uma das 2 réplicas do sync | p95 da reconexão < 5 s; nenhum ouvinte fica sem voltar |
| RES-02 | `docker kill` do worker no meio de uma conversão longa | outro worker conclui o job; a faixa aparece |
| RES-03 | YouTube inacessível (domínios → `127.0.0.1` no worker) | 3 retries → DLQ; o circuit breaker abre |
| RES-04 | `docker compose restart redis` com DJ tocando | nenhum ouvinte fica > 5 s sem evento |
| RES-05 | `docker compose stop postgres` | ouvintes seguem escutando; `POST /stations` falha em ≤ 10 s com 5xx |

## Estado da validação

Todos os cinco foram **executados contra o Compose completo** (resultados e
números em `docs/resultados-testes.md`, seção 4):

| Script | Resultado |
|---|---|
| RES-01 | passou: reconexão p95 = 504 ms, 0 ouvintes perdidos |
| RES-02 | passou (2 execuções): job `ready` 21 s depois do `kill -9`, concluído por um container novo |
| RES-03 | passou: 7 jobs `failed`, DLQ +7, 3 tentativas, breaker aberto |
| RES-04 | passou: 0 ouvintes perdidos, Redis voltou com os dados (AOF) |
| RES-05 | passou: `503` em 0 s com mensagem clara, ouvintes seguiram |

## Observações

- **Rate limit do gateway:** a criação de estação (10 por minuto) e o `/youtube` (6 por minuto, rajada de 3) respondem `429` quando excedidos. Os scripts esperam e tentam de novo; o RES-03 leva alguns minutos por isso.
- **RES-03** aplica `res-03.override.yml` (retries de 2/4/6 s, bloqueio do YouTube) e **restaura** a configuração normal ao terminar, mesmo se falhar.
- **RES-02** usa um áudio de 20 min gerado dentro do worker; o arquivo vai para `tests/resilience/results/` (ignorado pelo git, ~18 MB).
- Pela leitura do código (não testado): se a réplica do DJ cair, o lock de DJ no Redis só expira após 15 s, e uma nova conexão que chega antes disso entra como ouvinte. Por isso o RES-01 roda sem DJ.
- `COMPOSE_NETWORK=<rede>` se o projeto não se chama como a pasta; `GATEWAY_LOCAL=<url>` se o gateway não está em `http://localhost`.
