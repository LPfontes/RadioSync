#!/usr/bin/env bash
# RES-02 — Worker morto no meio de uma conversão.
# Esperado: a mensagem volta à fila (o ack é manual, então o broker devolve o que
# estava sem ack) e OUTRO worker conclui o job; a faixa aparece na estação.
#
#   tests/resilience/res-02-worker-morto.sh
source "$(dirname "$0")/lib.sh"; exige_stack
GATEWAY_LOCAL="${GATEWAY_LOCAL:-http://localhost}"
MIN="${DURACAO_AUDIO_S:-1200}" # áudio longo para a conversão durar o bastante para dar tempo de matar

echo ">> RES-02: um único worker, para saber quem está com o job"
docker compose up -d --scale media-worker=1 media-worker >/dev/null
sleep 8

echo ">> RES-02: gerando um áudio de $MIN s dentro do worker (FFmpeg)"
docker compose exec -T media-worker ffmpeg -loglevel error -f lavfi -i "sine=frequency=440:duration=$MIN" -b:a 128k -y /tmp/longo.mp3
mkdir -p "$SAIDA_DIR"
docker compose cp media-worker:/tmp/longo.mp3 "$SAIDA_DIR/longo.mp3" >/dev/null

cria_estacao
J=$(curl -s -X POST "$GATEWAY_LOCAL/api/v1/stations/$ID/upload" -H "Authorization: Bearer $TOK" -F "file=@$SAIDA_DIR/longo.mp3;type=audio/mpeg")
JOB=$(echo "$J" | python3 -c "import sys,json;print(json.load(sys.stdin)['jobId'])")
echo ">> RES-02: job $JOB na estação $ID"

# as rotas de job exigem o token de DJ; falha de rede vira "?" em vez de matar o script
estado() { curl -s -H "Authorization: Bearer $TOK" "$GATEWAY_LOCAL/api/v1/stations/$ID/jobs/$JOB" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('status','?'), d.get('attempt',''))" 2>/dev/null || echo "?"; }

for _ in $(seq 1 120); do
  s=$(estado); [ "${s%% *}" = "processing" ] && break; sleep 0.5
done
echo ">> RES-02: status antes de matar: $s"
[ "${s%% *}" = "processing" ] || { echo "RES-02: o job não chegou a 'processing' (status '$s')"; exit 1; }

t0=$(date +%s)
echo ">> RES-02: docker kill do worker no meio da conversão (SIGKILL, sem ack)"
docker kill "$(docker compose ps -q media-worker | head -1)" >/dev/null
echo ">> RES-02: subindo OUTRO worker (container novo)"
docker compose run -d --no-deps --name "res02-worker-$$" media-worker >/dev/null

for _ in $(seq 1 240); do
  s=$(estado); case "${s%% *}" in ready|failed) break;; esac; sleep 2
done
dur=$(( $(date +%s) - t0 ))
echo ">> RES-02: status final: $s (${dur}s depois do kill)"

faixa=$(curl -s -H "Authorization: Bearer $TOK" "$GATEWAY_LOCAL/api/v1/stations/$ID/jobs/$JOB" | python3 -c "import sys,json;t=json.load(sys.stdin).get('track') or {};print(t.get('title',''))" 2>/dev/null || true)
echo ">> RES-02: faixa gerada pelo job: '${faixa:-<nenhuma>}'"
docker rm -f "res02-worker-$$" >/dev/null 2>&1 || true

if [ "${s%% *}" = "ready" ] && [ -n "${faixa:-}" ]; then echo "RES-02: PASSOU"; else echo "RES-02: FALHOU"; exit 1; fi
