#!/usr/bin/env bash
# LOAD-02 — Mesmo cenário do LOAD-01 com 1, 2 e 4 réplicas do Sync Service.
# Métrica: conexões suportadas e uso de CPU. Meta: escala ~linear.
#
#   tests/load/load-02-replicas.sh                       # 1, 2 e 4 réplicas
#   REPLICAS="1 3" OUVINTES=300 tests/load/load-02-replicas.sh
#
# Para cada número de réplicas: escala o sync, roda o LOAD-01 e amostra o CPU
# dos containers do sync a cada 5 s. Imprime uma tabela no final.
set -euo pipefail
cd "$(dirname "$0")/../.."

replicas="${REPLICAS:-1 2 4}"
ouvintes="${OUVINTES:-500}"
saida="tests/load/results/load-02-$(date +%Y%m%d-%H%M%S).txt"
mkdir -p tests/load/results
: > "$saida"

for n in $replicas; do
  echo ">> LOAD-02: $n réplica(s) do sync, $ouvintes ouvintes"
  docker compose up -d --scale sync="$n" sync >/dev/null
  sleep 15 # o gateway descobre as réplicas novas pelo Docker

  amostras="$(mktemp)"
  ( while true; do
      docker stats --no-stream --format '{{.CPUPerc}}' $(docker compose ps -q sync) 2>/dev/null \
        | tr -d '%' | awk '{s+=$1} END {print s}' >> "$amostras"
      sleep 5
    done ) &
  amostrador=$!

  set +e
  tests/load/run.sh load-01 -e OUVINTES="$ouvintes" "$@" > "tests/load/results/load-02-${n}r.log" 2>&1
  codigo=$?
  set -e
  kill "$amostrador" 2>/dev/null || true; wait "$amostrador" 2>/dev/null || true

  cpu_media=$(awk '{s+=$1; c++} END {if (c) printf "%.1f", s/c; else print "n/d"}' "$amostras")
  cpu_pico=$(sort -n "$amostras" | tail -1)
  p95=$(grep -E "propagacao_ms\.*:" "tests/load/results/load-02-${n}r.log" | sed -E 's/.*p\(95\)=([^ ]+).*/\1/' | head -1)
  printf '%s réplicas | ouvintes=%s | p95 propagação=%s | CPU soma média=%s%% pico=%s%% | k6 exit=%s\n' \
    "$n" "$ouvintes" "${p95:-n/d}" "$cpu_media" "${cpu_pico:-n/d}" "$codigo" | tee -a "$saida"
  rm -f "$amostras"
done
echo ">> tabela salva em $saida"
