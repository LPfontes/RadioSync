#!/usr/bin/env sh
# Gera (ou troca) o usuário e a senha do dashboard do Traefik (ADR-0005).
#
# Uso:   ./scripts/traefik-dashboard-password.sh [usuario]     (padrão: admin)
#
# A senha é lida do terminal sem eco (não fica no histórico do shell) e salva
# com bcrypt em infra/traefik/dashboard.htpasswd, que está no .gitignore.
# Depois, reinicie o gateway para o Traefik ler o arquivo novo.
# Requer Docker (usa o htpasswd da imagem oficial do Apache httpd).
set -eu

user="${1:-admin}"
dir="$(cd "$(dirname "$0")/.." && pwd)"
out="$dir/infra/traefik/dashboard.htpasswd"

case "$user" in
  *:*|"") echo "usuário inválido: '$user'" >&2; exit 1 ;;
esac

# Desliga o eco antes de mostrar o prompt (senão o que for digitado logo em
# seguida aparece na tela) e garante que ele volte mesmo com Ctrl+C.
trap 'stty echo 2>/dev/null' EXIT INT TERM
stty -echo
printf 'Senha para "%s": ' "$user"
IFS= read -r pass; echo
printf 'Repita a senha: '
IFS= read -r pass2; echo
stty echo

if [ "$pass" != "$pass2" ]; then
  echo "as senhas não conferem" >&2; exit 1
fi
if [ "${#pass}" -lt 12 ]; then
  echo "use pelo menos 12 caracteres" >&2; exit 1
fi

line="$(printf '%s' "$pass" | docker run --rm -i httpd:2.4-alpine htpasswd -niBC 12 "$user")"
umask 077
printf '%s\n' "$line" > "$out"
echo "ok: $out atualizado (usuário: $user)"
echo "agora reinicie o gateway:  docker compose restart gateway"
