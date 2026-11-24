#!/usr/bin/env bash
# RES-01 — `docker stop` de uma réplica do Sync Service com ouvintes conectados.
# Esperado: os clientes reconectam em outra réplica e recebem o estado atual
# em < 5 s (p95), e nenhum ouvinte fica sem voltar.
#
#   tests/resilience/res-01-replica-sync.sh
source "$(dirname "$0")/lib.sh"; exige_stack

echo ">> RES-01: subindo 2 réplicas do sync"
docker compose up -d --scale sync=2 sync >/dev/null
sleep 15 # o gateway descobre as réplicas

sonda_inicia res-01 -e OUVINTES="${OUVINTES:-30}" -e DURACAO=50
espera 15
vitima=$(docker compose ps -q sync | head -1)
echo ">> RES-01: docker stop da réplica $(docker inspect -f '{{.Name}}' "$vitima")"
docker stop "$vitima" >/dev/null
espera 5
echo ">> RES-01: religando a réplica"
docker start "$vitima" >/dev/null

if sonda_espera; then echo "RES-01: PASSOU"; else echo "RES-01: FALHOU"; exit 1; fi
