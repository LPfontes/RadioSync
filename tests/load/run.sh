#!/usr/bin/env bash
# Roda um cenário k6 dentro da rede do Compose (mesmo relógio dos servidores).
#
#   tests/load/run.sh load-01                      # padrões do cenário
#   tests/load/run.sh load-01 -e OUVINTES=100      # variáveis do cenário
#
# Pré-requisito: a stack no ar (docker compose up -d --build).
# Variáveis de ambiente úteis:
#   COMPOSE_NETWORK  rede do Compose (padrão: <nome-da-pasta>_default)
#   BASE_URL         URL do gateway dentro da rede (padrão: http://gateway)
#   WS_BASE_URL      URL do WebSocket, se for diferente da BASE_URL (ex.: ws://sync:8082)
#   K6_IMAGE         imagem do k6 (padrão: grafana/k6:latest)
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "uso: $0 <load-01|load-03|load-04|load-05> [opções do k6, ex.: -e OUVINTES=100]" >&2
  exit 2
fi

cd "$(dirname "$0")/../.."
cenario="$1"; shift
script=$(ls tests/load/"$cenario"-*.js 2>/dev/null | head -1 || true)
[ -n "$script" ] || { echo "cenário não encontrado: $cenario" >&2; exit 2; }

rede="${COMPOSE_NETWORK:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]')_default}"
docker network inspect "$rede" >/dev/null 2>&1 || {
  echo "rede '$rede' não existe. A stack está no ar? (docker compose up -d --build)" >&2
  echo "Se o projeto tem outro nome, use COMPOSE_NETWORK=<rede>." >&2
  exit 2
}

mkdir -p tests/load/results
saida="tests/load/results/${cenario}-$(date +%Y%m%d-%H%M%S).json"

echo ">> $cenario  rede=$rede  base=${BASE_URL:-http://gateway}"
set +e
docker run --rm --network "$rede" -v "$PWD":/work -w /work \
  -e BASE_URL="${BASE_URL:-http://gateway}" \
  ${WS_BASE_URL:+-e WS_BASE_URL="$WS_BASE_URL"} \
  "${K6_IMAGE:-grafana/k6:latest}" run \
  --summary-export="/work/$saida" "$@" "/work/$script"
codigo=$?
set -e

echo ">> resumo exportado em $saida (código de saída do k6: $codigo)"
exit $codigo
