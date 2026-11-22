#!/usr/bin/env bash
# Deploy do app com imagem imutável, health gate e rollback automático (ADR-0007).
#
# Uso:
#   ./scripts/deploy.sh            # versão = SHA curto do HEAD (constrói a imagem se não existir)
#   ./scripts/deploy.sh <tag>      # versão já construída ou baixada (ex.: rollback manual)
#
# Passos:
#   1. resolve a tag (SHA do commit) e garante que a imagem <imagem>:<tag> existe
#   2. anota a versão que está no ar (para o rollback)
#   3. recria só o container do app com a imagem nova (docker compose up --no-deps)
#      O antigo recebe SIGTERM, salva o estado e sai; o novo carrega o estado.
#   4. espera /healthz responder 200 COM a versão nova
#   5. falhou ou estourou o tempo? volta para a versão anterior e sai com erro
#
# Variáveis opcionais:
#   RADIOSYNC_IMAGE  repositório da imagem (padrão: lpfonts/radiosync)
#   DEPLOY_TIMEOUT   segundos para a versão nova ficar saudável (padrão: 60)
#   ALLOW_DIRTY=1    permite construir com mudanças não commitadas (tag ganha -dirty)
set -euo pipefail

cd "$(dirname "$0")/.."

IMAGE="${RADIOSYNC_IMAGE:-lpfonts/radiosync}"
TIMEOUT="${DEPLOY_TIMEOUT:-60}"
SERVICE="radiosync"
CONTAINER="radiosync"
LOG_DIR="deploy"
LOG_FILE="$LOG_DIR/history.log"

now_ms() { python3 -c 'import time; print(int(time.time()*1000))' 2>/dev/null || echo $(( $(date +%s) * 1000 )); }
log() { printf '[deploy] %s\n' "$*"; }
record() {
  mkdir -p "$LOG_DIR"
  printf '%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" "$2" "$3" >> "$LOG_FILE"
}

# ---------------------------------------------------------------- 1. versão
HEAD_SHA="$(git rev-parse --short=12 HEAD)"
TAG="${1:-$HEAD_SHA}"
DIRTY=""
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then DIRTY=1; fi

if ! docker image inspect "$IMAGE:$TAG" >/dev/null 2>&1; then
  if [ "$TAG" != "$HEAD_SHA" ]; then
    log "imagem $IMAGE:$TAG não existe aqui; faça o build ou o pull dela antes"
    exit 1
  fi
  if [ -n "$DIRTY" ]; then
    if [ "${ALLOW_DIRTY:-}" != "1" ]; then
      log "há mudanças não commitadas: a imagem não corresponderia ao commit $HEAD_SHA"
      log "commite antes, ou use ALLOW_DIRTY=1 (a tag vira $HEAD_SHA-dirty)"
      exit 1
    fi
    TAG="$HEAD_SHA-dirty"
  fi
  log "construindo $IMAGE:$TAG"
  docker build -q -t "$IMAGE:$TAG" \
    --label "org.opencontainers.image.revision=$(git rev-parse HEAD)" \
    --label "org.opencontainers.image.version=$TAG" . >/dev/null
fi

# ---------------------------------------------------------- 2. versão no ar
PREV_TAG=""
if PREV_IMAGE="$(docker inspect "$CONTAINER" --format '{{.Config.Image}}' 2>/dev/null)"; then
  PREV_TAG="${PREV_IMAGE##*:}"
fi
log "no ar: ${PREV_TAG:-nenhuma}  →  nova: $TAG"
if [ "$PREV_TAG" = "$TAG" ]; then
  log "essa versão já está no ar; nada a fazer"
  exit 0
fi

# ------------------------------------------------- 3 e 4. troca e health gate
switch_to() {
  IMAGE_TAG="$1" docker compose up -d --no-deps "$SERVICE" >/dev/null 2>&1
}

# Espera /healthz = 200 com a versão pedida. Consulta por dentro do container,
# então funciona qualquer que seja a porta publicada.
wait_healthy() {
  local want="$1" deadline=$(( $(date +%s) + TIMEOUT )) body
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if body="$(docker exec "$CONTAINER" curl -fsS http://localhost:8080/healthz 2>/dev/null)" \
       && printf '%s' "$body" | grep -q "\"version\":\"$want\""; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

T0="$(now_ms)"
switch_to "$TAG"
if wait_healthy "$TAG"; then
  T1="$(now_ms)"
  log "ok: $TAG no ar e saudável; indisponibilidade do app: $(( T1 - T0 )) ms"
  record "$TAG" "ok" "$(( T1 - T0 ))ms (antes: ${PREV_TAG:-nenhuma})"
  exit 0
fi

# ---------------------------------------------------------------- 5. rollback
log "FALHOU: $TAG não ficou saudável em ${TIMEOUT}s. Últimas linhas do log:"
docker logs --tail 15 "$CONTAINER" 2>&1 | sed 's/^/    /' || true
record "$TAG" "falhou" "rollback para ${PREV_TAG:-nenhuma}"

if [ -z "$PREV_TAG" ]; then
  log "não havia versão anterior para voltar"
  exit 1
fi
log "rollback para $PREV_TAG"
T2="$(now_ms)"
switch_to "$PREV_TAG"
if wait_healthy "$PREV_TAG"; then
  log "rollback ok: $PREV_TAG de volta no ar em $(( $(now_ms) - T2 )) ms"
  record "$PREV_TAG" "rollback-ok" "depois da falha de $TAG"
else
  log "rollback TAMBÉM falhou; verifique o container manualmente"
  record "$PREV_TAG" "rollback-falhou" "depois da falha de $TAG"
fi
exit 1
