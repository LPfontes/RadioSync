#!/usr/bin/env bash
# Tira uma captura do dashboard do Grafana para um intervalo de tempo, com o
# Chrome em modo headless (sem plugin de renderização).
#
#   tests/load/captura-grafana.sh <saida.png> <inicio> <fim> [altura]
#   tests/load/captura-grafana.sh docs/img/resultados/load-01.png "2026-11-23 22:36:00" "2026-11-23 22:40:30"
#
# Com menos de ~60 s de espera os painéis saem vazios. Os horários são locais da máquina. Variáveis: GRAFANA_URL (padrão da stack
# isolada, http://localhost:13000, que tem acesso anônimo de leitura).
set -euo pipefail

[ $# -ge 3 ] || { echo "uso: $0 <saida.png> <inicio> <fim> [altura]" >&2; exit 2; }
saida="$1"; inicio="$2"; fim="$3"; altura="${4:-3400}"
chrome="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
[ -x "$chrome" ] || { echo "Chrome não encontrado em '$chrome' (defina CHROME=...)" >&2; exit 2; }

ms() { python3 -c "import sys,datetime as d;print(int(d.datetime.strptime(sys.argv[1],'%Y-%m-%d %H:%M:%S').astimezone().timestamp()*1000))" "$1"; }
de=$(ms "$inicio"); ate=$(ms "$fim")
url="${GRAFANA_URL:-http://localhost:13000}/d/radiosync-overview/radiosync-visao-geral?orgId=1&from=$de&to=$ate&refresh=&kiosk"

mkdir -p "$(dirname "$saida")"
"$chrome" --headless=new --disable-gpu --hide-scrollbars --window-size=1700,"$altura" \
  --virtual-time-budget=60000 --screenshot="$saida" "$url" >/dev/null 2>&1
echo "captura salva em $saida"
