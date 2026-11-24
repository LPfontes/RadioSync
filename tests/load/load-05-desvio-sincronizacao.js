// LOAD-05 — Desvio de sincronização entre N clientes simulados.
// Cada cliente pede SYNC_REQUEST a cada 2 s, como o frontend (ADR-0004), e
// compara a posição que o servidor informa com a posição esperada pelo estado
// do DJ, compensando metade do RTT. Meta: desvio p95 < 500 ms.
//
// "Diferença máxima entre dois clientes" <= 2 × desvio de cada um em relação à
// referência comum, então p95(desvio) < 500 ms implica < 1000 ms entre pares.
//
// Uso: tests/load/run.sh load-05 [-e CLIENTES=100 -e DURACAO=90]
import { Trend, Counter } from 'k6/metrics';
import { createStation, runDJ, openSocket } from './lib/common.js';

const CLIENTES = parseInt(__ENV.CLIENTES || '100');
const DURACAO = parseInt(__ENV.DURACAO || '90');
const INTERVALO_MS = parseInt(__ENV.SYNC_MS || '2000');

const desvio = new Trend('desvio_posicao_ms', true);
const rtt = new Trend('sync_rtt_ms', true);
const respostas = new Counter('sync_respostas');

export const options = {
  scenarios: {
    dj: { executor: 'per-vu-iterations', vus: 1, iterations: 1, maxDuration: `${DURACAO + 60}s`, exec: 'dj' },
    clientes: { executor: 'per-vu-iterations', vus: CLIENTES, iterations: 1, maxDuration: `${DURACAO + 60}s`, exec: 'cliente' },
  },
  thresholds: {
    desvio_posicao_ms: ['p(95)<500'],
    sync_respostas: ['count>0'],
  },
};

export function setup() {
  const est = createStation();
  console.log(`LOAD-05: estação ${est.stationId}, ${CLIENTES} clientes, ${DURACAO}s`);
  return { ...est, prazo: Date.now() + DURACAO * 1000 + 5000 };
}

// O DJ dá PLAY uma vez e deixa tocar; os clientes medem a posição corrente.
export function dj(d) { runDJ(d.stationId, d.djToken, 30000, d.prazo); }

export function cliente(d) {
  const ws = openSocket(d.stationId);
  let estado = null;
  let enviadoEm = 0;
  ws.onopen = () => {
    const t = setInterval(() => {
      if (Date.now() >= d.prazo) { clearInterval(t); ws.close(); return; }
      enviadoEm = Date.now();
      ws.send(JSON.stringify({ type: 'SYNC_REQUEST' }));
    }, INTERVALO_MS);
  };
  ws.onmessage = (ev) => {
    const recebidoEm = Date.now();
    let msg;
    try { msg = JSON.parse(ev.data); } catch (_) { return; }
    if (msg.type === 'STATE_UPDATE' && msg.state) { estado = msg.state; return; }
    if (msg.type !== 'SYNC' || !estado || !estado.isPlaying || !enviadoEm) return;
    const idaVolta = recebidoEm - enviadoEm;
    // Posição que o cliente passa a acreditar: a do servidor (calculada no
    // serverTime) mais o meio RTT que a resposta levou para chegar.
    const estimada = msg.position * 1000 + idaVolta / 2;
    // Posição "verdadeira" no mesmo instante, pelo estado que o DJ publicou.
    const esperada = estado.seekOffset * 1000 + (recebidoEm - estado.startedAt);
    desvio.add(Math.abs(estimada - esperada));
    rtt.add(idaVolta);
    respostas.add(1);
  };
}
