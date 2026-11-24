# Testes de carga (k6)

Cenários do plano de testes (`docs/plano-de-testes.md`, seção 3). Rodam **dentro
da rede do Compose**, num container do k6: assim o relógio do k6 é o mesmo dos
servidores e a latência de propagação (`Date.now() - state.startedAt`) não
carrega erro de relógio entre máquinas.

## Como rodar

```bash
docker compose up -d --build            # a stack precisa estar no ar
tests/load/run.sh load-01               # padrões do cenário
tests/load/run.sh load-01 -e OUVINTES=100 -e RAMPA=30
```

O `run.sh` usa a rede `<nome-da-pasta>_default` (para a pasta `RadioSync`, é
`radiosync_default`). Se o projeto tiver outro nome: `COMPOSE_NETWORK=<rede>`.
O resumo de cada execução fica em `tests/load/results/` (ignorado pelo git).
Os números que entram no relatório vão para `docs/resultados-testes.md`.

| ID | Script | O que faz | Meta |
|---|---|---|---|
| LOAD-01 | `load-01-rampa-ouvintes.js` | Rampa de 0 → 500 ouvintes numa estação; o DJ dá `PLAY` a cada 2 s e cada ouvinte mede a propagação | p95 < 200 ms |
| LOAD-02 | `load-02-replicas.sh` | LOAD-01 com 1, 2 e 4 réplicas do sync; amostra o CPU | escala ~linear |
| LOAD-03 | `load-03-uploads-concorrentes.js` | 200 ouvintes + 20 uploads simultâneos; compara p95 antes e durante | degradação < 20%; upload `202` < 500 ms |
| LOAD-04 | `load-04-rajada-criacao.js` | 50 req/s de `POST /stations` por 30 s | gateway devolve `429` no excedente, sem 5xx |
| LOAD-05 | `load-05-desvio-sincronizacao.js` | 100 clientes pedindo `SYNC_REQUEST` a cada 2 s | desvio de posição p95 < 500 ms |

As metas viram *thresholds* do k6: o processo sai com código ≠ 0 se alguma falhar.
No JSON exportado (`--summary-export`), `false` ao lado de um threshold significa
que ele **não** falhou.

## Observações de cada cenário

- **LOAD-04 esgota a cota de criação de estação** (10 req/min por IP) por cerca
  de 1 min. Rode por último. Os outros cenários criam a estação em `setup()` e
  esperam se o gateway responder `429`.
- **LOAD-05** define o desvio como `|posição do servidor + RTT/2 − posição
  esperada pelo estado do DJ|`. A diferença entre dois clientes quaisquer é no
  máximo o dobro disso; p95 < 500 ms implica < 1 s entre pares.
- **LOAD-03** usa `frontend/public/sounds/alarm.mp3` como arquivo de upload. Pelo gateway, 20 uploads de um mesmo IP batem no limite de taxa (6 por minuto) e quase todos recebem `429`; para medir o efeito da **fila** sobre o WebSocket, mande direto à Media API: `tests/load/run.sh load-03 -e UPLOAD_BASE_URL=http://media:8084`.
- `WS_BASE_URL=ws://sync:8082` aponta o WebSocket direto para o Sync Service,
  sem passar pelo gateway (útil para isolar o gateway como gargalo).

## Limites conhecidos

- O k6 roda numa única máquina, junto com a stack. Em 500 ouvintes, parte da
  latência medida pode ser do próprio k6 disputando CPU com os servidores.
- O monólito e o Sync Service compartilham o `JWT_SECRET`; sem ele os tokens de
  DJ do `setup()` não serão aceitos.


## Capturas do Grafana

```bash
tests/load/captura-grafana.sh docs/img/resultados/x.png "2026-11-24 01:23:50" "2026-11-24 01:27:05"
```

Usa o Chrome em modo headless (sem plugin de renderização) e o Grafana da stack
isolada (porta 13000, acesso anônimo de leitura). Termine a janela um pouco
antes do fim do teste, senão os indicadores do topo mostram 0 conexões.

## Cuidados

- **No macOS, rode sob `caffeinate`** (`caffeinate -dimsu tests/load/run.sh load-01`). O repouso do sistema congela os timers do k6: um LOAD-01 de 3 minutos virou 32 minutos com 43% de erros e precisou ser descartado.
- Os arquivos de `tests/load/results/` podem conter tokens de DJ; por isso o diretório está no `.gitignore`.
