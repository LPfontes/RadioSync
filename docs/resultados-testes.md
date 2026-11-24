# Resultados dos testes

Documento vivo da frente de Observabilidade e Testes (issue #19). Compara o que
foi **medido** com as metas do [plano de testes](plano-de-testes.md). Está
dividido em três níveis de confiança, para ninguém confundir o que foi rodado
com o que ainda vai ser:

- **Medido:** executado e com número registrado.
- **Fumaça:** executado em escala pequena, numa mini-stack isolada (veja a
  seção 1). Prova que o teste funciona e dá uma ordem de grandeza, mas **não
  substitui a rodada completa**.
- **Pendente:** ainda não executado.

Última atualização: 24/11/2026, sobre a `dev` no commit `b3c392f`.

## 0. Resumo contra as metas

| ID | Meta | Resultado | Nível |
|---|---|---|---|
| Cobertura | ≥ 50% em `auth`, `model`, `ws` | auth 92,4% · model 95,7% · ws 86,0% | **Medido** ✅ |
| INT/RES (Go) | passam contra a infra | 11 de 11 passam (Postgres, Redis e RabbitMQ reais) | **Medido** ✅ — mas 6 deles usam fakes em memória (seção 3) |
| RES-01 | reconexão < 5 s | p95 = 2,39 s, 0 ouvintes perdidos | **Fumaça** ✅ |
| RES-04 | eventos voltam a fluir | 0 ouvintes perdidos, último evento há 1,95 s | **Fumaça** ✅ |
| LOAD-01 | propagação p95 < 200 ms | p95 = 3 ms com **50** ouvintes | **Fumaça** ✅ (a meta é para 500) |
| LOAD-04 | gateway barra o excedente com 429 | 992 de 1000 respostas 429, 0 erros 5xx | **Fumaça** ✅ (Traefik de teste, não o do Compose) |
| LOAD-05 | desvio p95 < 500 ms | p95 = 1 ms com 100 clientes | **Fumaça** ✅ |
| LOAD-02 | escala ~linear com 1/2/4 réplicas | — | **Pendente** |
| LOAD-03 | degradação < 20% com uploads | — | **Pendente** |
| RES-02, RES-03 | worker morto; yt-dlp falhando | passam em Go com RabbitMQ real | **Medido** (sem versão com `docker stop`) |
| RES-05 | Postgres fora | passa em Go (fake em memória) | **Pendente** com Postgres real |
| INT-03 (gateway) | rotas admin sem token → 401 | testado no Go; ForwardAuth do Traefik **não** testado | **Parcial** |

## 1. Ambiente e método

- **Máquina:** MacBook Air (Apple Silicon, `arm64`), Docker Desktop (VM com ~7,7 GiB de RAM).
- **k6** v2.4.0 em container, **dentro da mesma rede** dos serviços, então o relógio do k6 é o dos servidores (a latência de propagação é `Date.now() - state.startedAt`, sem erro de relógio entre máquinas).
- **Mini-stack de fumaça:** Redis 7 (com AOF), Sync Service (1 ou 2 réplicas), monólito e, quando preciso, um Traefik v3.6 configurado por arquivo com os mesmos limites de taxa do `docker-compose.yml`. Foi montada à parte porque a stack completa do Compose estava ocupando os mesmos nomes de container e portas.
- **Carga e CPU:** o k6 roda na mesma máquina dos servidores, então em cargas altas parte da latência pode ser disputa de CPU.

## 2. Testes unitários e cobertura

`go test -cover ./...` (com `-race` nos três pacotes da meta):

| Pacote | Cobertura | | Pacote | Cobertura |
|---|---|---|---|---|
| `security` | 100,0% | | `queue` | 41,3% |
| `model` | **95,7%** | | `stationservice` | 45,9% |
| `auth` | **92,4%** | | `storage` | 41,0% |
| `mediaservice` | 90,6% | | `jobs` | 43,7% |
| `breaker` | 90,0% | | `handler` | 29,4% |
| `mediaworker` | 86,7% | | `syncservice` | 49,9% |
| `ws` | **86,0%** | | `importer` | 65,8% |
| `media` | 71,9% | | `metrics` | 71,1% |

Os `cmd/*` (pontos de entrada) não têm teste. Antes do trabalho desta frente,
`model` estava em 8,6% e `ws` em 43,0%.

Um bug real foi achado ao escrever os testes de `ws`: o `Hub.Run` removia
clientes do mapa segurando só `RLock` (corrida de dados com `ClientCount` e
`HasDJConnected`). Corrigido no PR #34.

## 3. Integração e resiliência em Go

`tests/integration/run.sh` roda os 11 testes contra Postgres 16, Redis 7 e
RabbitMQ 3.13 reais. **Todos passam.** O que cada um usa de verdade:

| Teste | Infra real? |
|---|---|
| INT-02, RES-02, RES-03 | **RabbitMQ real** |
| INT-01, INT-04, INT-05, RES-01, RES-04, RES-05 | **Fakes em memória** (`MemoryBroker`, `MemoryRepository`) |
| INT-03 | httptest, sem infra |

O plano de testes prometia infra real em todos. Os seis testes em memória
validam a lógica (fan-out, reconexão, consistência), mas **não provam
sobrevivência a uma falha real**. Por exemplo, o `RES-04` "reinicia o Redis"
refazendo a assinatura num broker em memória.

## 4. Resiliência com falha real (`docker stop`)

Mini-stack isolada, sonda k6 com ouvintes que reconectam sozinhos.

| Cenário | Falha | Resultado | Meta |
|---|---|---|---|
| **RES-01** | `docker stop` de 1 das 2 réplicas do sync (30 ouvintes, 50 s) | 15 de 30 ouvintes caíram; **todos reconectaram**; reconexão média 1,08 s, **p95 = 2,39 s**, máx. 2,75 s | p95 < 5 s ✅ |
| **RES-04** | `docker restart` do Redis com DJ tocando (30 ouvintes, 50 s) | 0 quedas de conexão; último evento há 1,95 s ao fim; AOF manteve as chaves | < 5 s sem evento ✅ |

Limites: um único `stop` por execução e uma só execução de cada; sem repetição
para medir variância. **RES-05** (Postgres) não foi executado.

## 5. Carga (testes de fumaça)

Cargas pequenas, só para validar os scripts e dar ordem de grandeza.

| Cenário | Configuração | Resultado | Meta |
|---|---|---|---|
| **LOAD-01** | 50 ouvintes, rampa de 15 s + 15 s | 684 propagações; **p95 = 3 ms** (média 1,6 ms, máx. 3 ms); 0 erros de WebSocket | p95 < 200 ms |
| **LOAD-04** | 50 req/s por 20 s no `POST /stations` | 1000 requisições: **8 criaram**, **992 levaram 429 (99,2%)**, 0 erros 5xx | barrar o excedente, sem 5xx |
| **LOAD-05** | 100 clientes, `SYNC_REQUEST` a cada 2 s, 30 s | 1700 respostas; **desvio p95 = 1 ms** (máx. 2 ms); RTT p95 = 3 ms | desvio p95 < 500 ms |

**Isto não é a rodada da meta.** O LOAD-01 pede 500 ouvintes e aqui foram 50. O
resultado de 3 ms também é de uma máquina só, sem rede real entre cliente e
servidor. O LOAD-04 foi contra um Traefik de teste com os limites copiados do
Compose, não contra o gateway real.

## 6. CI

Na `dev` (`b3c392f`) os workflows **CI** e **Security** passam. Durante o
trabalho o Security estava vermelho por duas causas, ambas corrigidas:

1. **GO-2026-6278** em `gorilla/websocket` v1.5.3 (PRNG fraco na máscara do WebSocket), alcançável pelo Sync Service. Atualizado (PR #32). A correção só existe como commit do upstream (sem release), então o `go.mod` usa uma pseudo-versão.
2. A atualização acima puxou `golang.org/x/net` v0.59.0, com **CVE-2026-78661** e **CVE-2026-78669** (HIGH) apontados pelo Trivy nas imagens. Subido para v0.63.0.

## 7. Pendências (para a rodada completa)

Precisam da stack completa do Compose livre, com a nossa execução isolada:

1. **LOAD-01 completo:** 500 ouvintes, com os painéis do Grafana capturados.
2. **LOAD-02:** 1, 2 e 4 réplicas do sync (`tests/load/load-02-replicas.sh`).
3. **LOAD-03:** 200 ouvintes + 20 uploads (precisa de fila, MinIO e worker).
4. **LOAD-04** contra o gateway real do Compose.
5. **RES-05** com Postgres real, e os scripts de `tests/resilience/` contra o Compose.
6. **Gráficos do Grafana** (p95 por rota, conexões WebSocket, fila, breaker, 429) durante essas execuções, para entrar aqui.
7. Teste do **ForwardAuth do Traefik** no INT-03.

## 8. Como reproduzir

```bash
# unitários e cobertura
cd backend && go test -race -cover ./...

# integração e RES em Go, contra infra real descartável
tests/integration/run.sh

# com a stack no ar (docker compose up -d --build)
tests/load/run.sh load-01            # LOAD-01 a LOAD-05; veja tests/load/README.md
tests/resilience/res-01-replica-sync.sh
```
