#!/usr/bin/env bash
# Derruba a stack isolada e apaga os volumes dela. Não toca na stack normal.
set -euo pipefail
cd "$(dirname "$0")/../.."
export COMPOSE_PROJECT_NAME="${STACK_PROJETO:-iso}"
export COMPOSE_FILE="docker-compose.yml:tests/stack-isolada/override.yml"
export IMAGE_TAG="${IMAGE_TAG:-iso}"
docker compose down -v --remove-orphans
