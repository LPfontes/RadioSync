#!/usr/bin/env bash
# RES-04 — Redis reiniciado.
# Esperado: o Sync Service reconecta, os eventos voltam a fluir e o estado é
# recuperado (AOF). Com DJ ativo, nenhum ouvinte pode ficar > 5 s sem evento.
#
#   tests/resilience/res-04-redis.sh
source "$(dirname "$0")/lib.sh"; exige_stack

sonda_inicia res-04 -e OUVINTES="${OUVINTES:-30}" -e DURACAO=50 -e DJ=1
espera 15
echo ">> RES-04: docker restart do redis"
docker compose restart redis >/dev/null
echo ">> RES-04: estado do Redis depois de reiniciar (AOF)"
sleep 5; docker compose exec -T redis redis-cli dbsize || true

if sonda_espera; then echo "RES-04: PASSOU"; else echo "RES-04: FALHOU"; exit 1; fi
