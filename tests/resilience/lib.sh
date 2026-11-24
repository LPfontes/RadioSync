#!/usr/bin/env bash
# Funções comuns dos testes de resiliência (compose-level, com falha real).
# Uso: source "$(dirname "$0")/lib.sh"
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

REDE="${COMPOSE_NETWORK:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]')_default}"
K6_IMAGE="${K6_IMAGE:-grafana/k6:latest}"
SAIDA_DIR="tests/resilience/results"
mkdir -p "$SAIDA_DIR"

exige_stack() {
  docker network inspect "$REDE" >/dev/null 2>&1 || {
    echo "rede '$REDE' não existe. A stack está no ar? (docker compose up -d --build)" >&2
    echo "Se o projeto tem outro nome, use COMPOSE_NETWORK=<rede>." >&2
    exit 2
  }
}

# Roda a sonda do k6 em segundo plano; guarda o PID em $K6_PID e o log em $K6_LOG.
sonda_inicia() {
  local nome="$1"; shift
  K6_LOG="$SAIDA_DIR/${nome}-$(date +%Y%m%d-%H%M%S).log"
  docker run --rm --network "$REDE" -v "$PWD":/work -w /work \
    -e BASE_URL="${BASE_URL:-http://gateway}" ${WS_BASE_URL:+-e WS_BASE_URL="$WS_BASE_URL"} \
    "$K6_IMAGE" run --quiet "$@" /work/tests/resilience/probe-ouvintes.js >"$K6_LOG" 2>&1 &
  K6_PID=$!
}

# Espera a sonda terminar e imprime o resultado. Sai com o código do k6.
sonda_espera() {
  local codigo=0
  wait "$K6_PID" || codigo=$?
  echo "---- resultado da sonda ($K6_LOG) ----"
  grep -E "reconexao_ms|quedas_de_conexao|ouvintes_sem_reconectar|ultimo_update_atras_ms|✓|✗|threshold|ERRO|error" "$K6_LOG" | sed 's/\x1b\[[0-9;]*m//g' | head -20 || true
  return $codigo
}

# Dá a falha no meio da sonda: espera N s depois do início.
espera() { sleep "$1"; }
