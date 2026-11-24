# Testes de integração (INT) e resiliência em Go (RES)

```bash
tests/integration/run.sh                  # todos os INT e RES
RUN='TestINT02' tests/integration/run.sh  # só um
```

O `run.sh` sobe Postgres 16, Redis 7 (com AOF) e RabbitMQ 3.13 em containers
`radiosync-it-*`, numa rede própria e sem publicar portas, então **não conflita
com uma stack já rodando**. Tudo é removido no fim, mesmo se o teste falhar.
No CI os mesmos testes rodam no job *Backend (Go)* com os serviços do GitHub
Actions (`TEST_DATABASE_URL`, `TEST_REDIS_URL`, `TEST_AMQP_URL`); sem essas
variáveis eles ficam em `SKIP`, por isso um `go test ./...` puro não os exercita.

Não usamos `-tags integration`: os testes são protegidos pelas variáveis
`TEST_*`, o que dispensa mexer nos arquivos das outras frentes.

## O que cada teste usa de verdade

Esta tabela existe porque o plano de testes diz "Postgres, Redis e RabbitMQ
reais", mas **só parte dos testes fala com a infraestrutura real**.

| ID | Teste (Go) | Infra usada |
|---|---|---|
| INT-01 | `syncservice.TestINT01_PlayFanoutBetweenReplicas` | **memória** (`MemoryBroker`) |
| INT-02 | `mediaworker.TestINT02_UploadFilaWorkerTrackReady` | **RabbitMQ real** |
| INT-03 | `handler.TestRotasAdmin_SemToken401` e `_TokenDeDJ401` | httptest (sem infra) |
| INT-04 | `stationservice.TestINT04_ReplicaStationConsistency` | **memória** (`MemoryRepository`) |
| INT-05 | `stationservice.TestINT05_StranglerFigContractParity` | **memória** |
| RES-01 | `syncservice.TestRES01_ReplicaFailoverReconnection` | **memória** |
| RES-02 | `mediaworker.TestRES02_WorkerMortoNoMeioDaConversao` | **RabbitMQ real** |
| RES-03 | `mediaworker.TestRES03_YtDlpFalhandoSempre` | **RabbitMQ real** |
| RES-04 | `syncservice.TestRES04_RedisRestart` | **memória** (o "reinício" só refaz a assinatura no broker em memória) |
| RES-05 | `stationservice.TestRES05_PostgresDown_…` | **memória** (repositório que simula a queda) |

Os testes em memória validam a **lógica** (fan-out, reconexão, consistência),
mas não provam que o sistema sobrevive a uma falha de infraestrutura de
verdade. Essa parte é coberta pelos scripts de `tests/resilience/`, que
derrubam containers reais.

O INT-03 é validado também na camada do gateway (ForwardAuth do Traefik): isso
só dá para conferir com a stack completa, chamando as rotas de `/api/v1/admin/*`
pelo gateway sem token e esperando `401` (ou `503` se `ADMIN_PASSWORD` não
estiver definido).
