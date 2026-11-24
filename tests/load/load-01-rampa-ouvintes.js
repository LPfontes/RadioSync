// LOAD-01 — Rampa de 0 → N ouvintes WebSocket numa estação.
// Métrica: latência de propagação do PLAY (p95). Meta: < 200 ms.
//
// Uso: tests/load/run.sh load-01 [-e OUVINTES=500 -e RAMPA=120 -e SUSTENTA=60]
import { Trend, Rate, Counter } from 'k6/metrics';
import { createStation, runDJ, runListener } from './lib/common.js';

const OUVINTES = parseInt(__ENV.OUVINTES || '500');
const RAMPA = parseInt(__ENV.RAMPA || '120'); // segundos subindo de 0 a OUVINTES
const SUSTENTA = parseInt(__ENV.SUSTENTA || '60'); // segundos no pico
const SONDA_MS = parseInt(__ENV.SONDA_MS || '2000'); // intervalo entre PLAY do DJ

const propagacao = new Trend('propagacao_ms', true);
const recebidos = new Counter('state_updates_recebidos');
const erros = new Rate('ws_erros');

export const options = {
  scenarios: {
    dj: { executor: 'per-vu-iterations', vus: 1, iterations: 1, maxDuration: `${RAMPA + SUSTENTA + 60}s`, exec: 'dj' },
    ouvintes: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [{ duration: `${RAMPA}s`, target: OUVINTES }, { duration: `${SUSTENTA}s`, target: OUVINTES }],
      gracefulRampDown: '15s',
      exec: 'ouvinte',
    },
  },
  thresholds: {
    propagacao_ms: ['p(95)<200'],
    ws_erros: ['rate<0.01'],
    state_updates_recebidos: ['count>0'],
  },
};

export function setup() {
  const est = createStation();
  const prazo = Date.now() + (RAMPA + SUSTENTA) * 1000 + 5000;
  console.log(`LOAD-01: estação ${est.stationId}, ${OUVINTES} ouvintes, rampa ${RAMPA}s + ${SUSTENTA}s`);
  return { stationId: est.stationId, djToken: est.djToken, prazo };
}

export function dj(d) { runDJ(d.stationId, d.djToken, SONDA_MS, d.prazo); }

export function ouvinte(d) {
  runListener(d.stationId, d.prazo,
    (agora, estado) => { propagacao.add(agora - estado.startedAt); recebidos.add(1); erros.add(0); },
    () => erros.add(1));
}
