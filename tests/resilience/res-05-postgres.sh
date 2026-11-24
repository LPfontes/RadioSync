#!/usr/bin/env bash
# RES-05 — Postgres indisponível.
# Esperado: ouvintes continuam escutando (o estado quente está no Redis) e a
# criação de estação falha com erro claro, sem travar.
#
#   tests/resilience/res-05-postgres.sh
source "$(dirname "$0")/lib.sh"; exige_stack

GATEWAY_LOCAL="${GATEWAY_LOCAL:-http://localhost}"

sonda_inicia res-05 -e OUVINTES="${OUVINTES:-30}" -e DURACAO=50 -e DJ=1
espera 15
echo ">> RES-05: docker stop do postgres"
docker compose stop postgres >/dev/null
sleep 5

# criar estação precisa do Postgres: tem que falhar rápido e com resposta legível
inicio=$(date +%s)
codigo=$(curl -s -o /tmp/res05-corpo -w '%{http_code}' --max-time 15 -X POST "$GATEWAY_LOCAL/api/v1/stations" -H 'Content-Type: application/json' -d '{}' || true)
dur=$(( $(date +%s) - inicio ))
echo ">> RES-05: POST /stations com Postgres fora -> HTTP $codigo em ${dur}s: $(head -c 120 /tmp/res05-corpo 2>/dev/null)"
falhou=0
case "$codigo" in 5??|429) ;; *) echo "   esperado erro 5xx claro, veio '$codigo'"; falhou=1;; esac
[ "$dur" -le 10 ] || { echo "   a resposta demorou ${dur}s (> 10 s)"; falhou=1; }

echo ">> RES-05: religando o postgres"
docker compose start postgres >/dev/null

if sonda_espera && [ "$falhou" = 0 ]; then echo "RES-05: PASSOU"; else echo "RES-05: FALHOU"; exit 1; fi
