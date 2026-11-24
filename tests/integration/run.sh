#!/usr/bin/env bash
# Harness de integração: roda os testes INT-xx e RES-xx do backend contra
# Postgres, Redis e RabbitMQ REAIS, em containers próprios e descartáveis.
#
#   tests/integration/run.sh                 # todos os INT e RES
#   RUN='TestINT02' tests/integration/run.sh # só os que casam com o padrão
#
# Não usa o Compose do projeto: sobe `radiosync-it-*` numa rede própria, sem
# publicar portas, então não conflita com uma stack que já esteja rodando. Tudo
# é removido no final (inclusive se o teste falhar).
#
# Os testes leem TEST_DATABASE_URL, TEST_REDIS_URL e TEST_AMQP_URL e ficam em
# SKIP quando elas não existem (é por isso que `go test ./...` puro não os roda).
set -euo pipefail
cd "$(dirname "$0")/../.."

REDE="radiosync-it"
PADRAO="${RUN:-TestINT|TestRES}"

limpa() { docker rm -f radiosync-it-pg radiosync-it-rd radiosync-it-mq >/dev/null 2>&1 || true; docker network rm "$REDE" >/dev/null 2>&1 || true; }
trap limpa EXIT
limpa

docker network create "$REDE" >/dev/null
docker run -d --name radiosync-it-pg --network "$REDE" \
  -e POSTGRES_USER=radiosync -e POSTGRES_PASSWORD=radiosync -e POSTGRES_DB=radiosync postgres:16-alpine >/dev/null
docker run -d --name radiosync-it-rd --network "$REDE" redis:7-alpine redis-server --appendonly yes >/dev/null
docker run -d --name radiosync-it-mq --network "$REDE" rabbitmq:3.13-alpine >/dev/null

echo ">> esperando Postgres, Redis e RabbitMQ ficarem prontos..."
for _ in $(seq 1 60); do
  if docker exec radiosync-it-pg pg_isready -U radiosync >/dev/null 2>&1 \
     && docker exec radiosync-it-rd redis-cli ping >/dev/null 2>&1 \
     && docker exec radiosync-it-mq rabbitmq-diagnostics -q ping >/dev/null 2>&1; then
    pronto=1; break
  fi
  sleep 3
done
[ "${pronto:-0}" = 1 ] || { echo "a infraestrutura não ficou pronta a tempo" >&2; exit 1; }

echo ">> go test -run '$PADRAO'"
docker run --rm --network "$REDE" -v "$PWD":/src -w /src/backend \
  -e TEST_DATABASE_URL='postgres://radiosync:radiosync@radiosync-it-pg:5432/radiosync?sslmode=disable' \
  -e TEST_REDIS_URL='redis://radiosync-it-rd:6379/0' \
  -e TEST_AMQP_URL='amqp://guest:guest@radiosync-it-mq:5672/' \
  golang:1.26 go test -count=1 -v -run "$PADRAO" ./internal/... 2>&1 \
  | grep -E '^(--- |ok  |FAIL|panic)' | grep -v 'no tests to run'
codigo=${PIPESTATUS[0]}
exit "$codigo"
