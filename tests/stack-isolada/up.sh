#!/usr/bin/env bash
# Sobe a stack COMPLETA do Compose ao lado de outra que já esteja rodando.
#
#   tests/stack-isolada/up.sh          # sobe (projeto "iso")
#   tests/stack-isolada/down.sh        # derruba e apaga volumes
#
# Depois use os mesmos scripts de teste, apontando para o projeto isolado:
#   export COMPOSE_PROJECT_NAME=iso COMPOSE_FILE=docker-compose.yml:tests/stack-isolada/override.yml
#   export COMPOSE_NETWORK=iso_default GATEWAY_LOCAL=http://localhost:18080
#   tests/load/run.sh load-04
#
# Portas do host: gateway 18080, Prometheus 19090, Grafana 13000, Jaeger 16687.
set -euo pipefail
cd "$(dirname "$0")/../.."

PROJETO="${STACK_PROJETO:-iso}"
[ -f .env ] || { echo "falta o .env (cp .env.example .env e defina o JWT_SECRET)" >&2; exit 2; }

docker compose config --format json | python3 tests/stack-isolada/gen-override.py "$PROJETO" > tests/stack-isolada/override.yml

export COMPOSE_PROJECT_NAME="$PROJETO"
export COMPOSE_FILE="docker-compose.yml:tests/stack-isolada/override.yml"
export IMAGE_TAG="${IMAGE_TAG:-iso}" # não sobrescreve as imagens :latest da stack normal
docker compose up -d --build "$@"
echo ">> stack '$PROJETO' no ar. Gateway: http://localhost:18080  Grafana: http://localhost:13000"
