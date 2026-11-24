#!/usr/bin/env bash
# RES-03 — yt-dlp falhando sempre (YouTube inacessível).
# Esperado: cada job faz 3 retries e vai para a DLQ; depois de 5 falhas o circuit
# breaker abre (estado 2) e passa a recusar sem tentar.
#
# O YouTube é "bloqueado" apontando os domínios dele para 127.0.0.1 no worker
# (res-03.override.yml), o que dá uma falha de rede determinística.
#
#   tests/resilience/res-03-ytdlp.sh
source "$(dirname "$0")/lib.sh"; exige_stack
GATEWAY_LOCAL="${GATEWAY_LOCAL:-http://localhost}"
BASE_FILES="${COMPOSE_FILE:-docker-compose.yml}"
OVERRIDE="tests/resilience/res-03.override.yml"
N="${JOBS:-7}" # acima do limite do breaker (5)

restaura() {
  echo ">> RES-03: restaurando a configuração normal do media e do worker"
  COMPOSE_FILE="$BASE_FILES" docker compose up -d --force-recreate media media-worker >/dev/null 2>&1 || true
}
trap restaura EXIT

echo ">> RES-03: aplicando o YouTube bloqueado + retries curtos (2s,4s,6s)"
COMPOSE_FILE="$BASE_FILES:$OVERRIDE" docker compose up -d --force-recreate --scale media-worker=1 media media-worker >/dev/null
sleep 12

dlq_msgs() { docker compose exec -T rabbitmq rabbitmqctl list_queues name messages 2>/dev/null | awk '$1 ~ /dlq$/ {print $2}' | head -1; }
DLQ_ANTES=$(dlq_msgs); DLQ_ANTES=${DLQ_ANTES:-0}
echo ">> RES-03: mensagens na DLQ antes: $DLQ_ANTES"
cria_estacao

# O gateway limita /youtube (media-ratelimit: 6 por minuto, rajada de 3, por IP),
# então os jobs entram com espera e novo envio quando vier 429.
JOBS_IDS=()
for i in $(seq 1 "$N"); do
  vid=$(printf 'AAAAAAAA%03d' "$i") # ids com 11 caracteres, todos inexistentes
  for _ in $(seq 1 12); do
    J=$(curl -s -w '\n%{http_code}' -X POST "$GATEWAY_LOCAL/api/v1/stations/$ID/youtube" -H "Authorization: Bearer $TOK" \
        -H 'Content-Type: application/json' -d "{\"url\":\"https://www.youtube.com/watch?v=$vid\"}" || true)
    [ "$(echo "$J" | tail -1)" = "429" ] || break
    sleep 10
  done
  JOBS_IDS+=("$(echo "$J" | head -1 | python3 -c "import sys,json;print(json.load(sys.stdin).get('jobId',''))" 2>/dev/null || true)")
done
echo ">> RES-03: $N jobs enfileirados"

estados() {
  for j in "${JOBS_IDS[@]}"; do
    curl -s -H "Authorization: Bearer $TOK" "$GATEWAY_LOCAL/api/v1/stations/$ID/jobs/$j" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('status','?'))" 2>/dev/null || echo "?"
  done | sort | uniq -c | tr '\n' ' '
}
for _ in $(seq 1 90); do
  e=$(estados)
  # termina quando nenhum job está mais em andamento
  echo "$e" | grep -qE "queued|processing|retrying" || break
  sleep 3
done
echo ">> RES-03: estado dos jobs: $e"

DLQ_DEPOIS=$(dlq_msgs); DLQ_DEPOIS=${DLQ_DEPOIS:-0}
DLQ_NOVAS=$(( DLQ_DEPOIS - DLQ_ANTES ))
estado_breaker=$(docker compose exec -T media-worker wget -qO- http://localhost:8085/metrics 2>/dev/null | awk '/^radiosync_media_breaker_state/ {print $2}' | head -1)
tentativas=$(curl -s -H "Authorization: Bearer $TOK" "$GATEWAY_LOCAL/api/v1/stations/$ID/jobs/${JOBS_IDS[0]}" | python3 -c "import sys,json;d=json.load(sys.stdin);print('tentativa',d.get('attempt'),'-',d.get('status'),'-',d.get('error',''))" 2>/dev/null || echo "?")
echo ">> RES-03: 1º job: $tentativas"
echo ">> RES-03: mensagens novas na DLQ: $DLQ_NOVAS (esperado: $N)"
echo ">> RES-03: estado do circuit breaker (0 fechado, 1 meio-aberto, 2 aberto): ${estado_breaker:-?}"

falhou=0
echo "$e" | grep -q "failed" || { echo "   nenhum job terminou em 'failed'"; falhou=1; }
[ "$DLQ_NOVAS" -ge "$N" ] || { echo "   a DLQ recebeu $DLQ_NOVAS mensagens, esperado pelo menos $N"; falhou=1; }
[ "${estado_breaker:-0}" = "2" ] || [ "${estado_breaker:-0}" = "2.0" ] || { echo "   o circuit breaker não abriu"; falhou=1; }

if [ "$falhou" = 0 ]; then echo "RES-03: PASSOU"; else echo "RES-03: FALHOU"; exit 1; fi
