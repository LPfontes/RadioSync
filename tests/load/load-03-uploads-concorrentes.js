// LOAD-03 — 20 uploads simultâneos enquanto 200 ouvintes escutam.
// Métrica: p95 da propagação no WebSocket antes vs. durante os uploads.
// Meta: degradação < 20% com a fila (ADR-0002); resposta do upload 202 em < 500 ms.
//
// Uso: tests/load/run.sh load-03 [-e OUVINTES=200 -e UPLOADS=20 -e ANTES=60 -e DURANTE=90]
import http from 'k6/http';
import { check } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';
import { createStation, runDJ, runListener, BASE_URL } from './lib/common.js';

const OUVINTES = parseInt(__ENV.OUVINTES || '200');
const UPLOADS = parseInt(__ENV.UPLOADS || '20');
const ANTES = parseInt(__ENV.ANTES || '60'); // segundos só com ouvintes (linha de base)
const DURANTE = parseInt(__ENV.DURANTE || '90'); // segundos observando depois dos uploads
const RAMPA = 30;
const SONDA_MS = parseInt(__ENV.SONDA_MS || '2000');

const arquivo = open('../../frontend/public/sounds/alarm.mp3', 'b');

const propagacao = new Trend('propagacao_ms', true);
const upload = new Trend('upload_resposta_ms', true);
const uploadAceito = new Rate('upload_aceito');
const recebidos = new Counter('state_updates_recebidos');
const erros = new Rate('ws_erros');

export const options = {
  scenarios: {
    dj: { executor: 'per-vu-iterations', vus: 1, iterations: 1, maxDuration: `${RAMPA + ANTES + DURANTE + 60}s`, exec: 'dj' },
    ouvintes: {
      executor: 'ramping-vus', startVUs: 0, gracefulRampDown: '15s', exec: 'ouvinte',
      stages: [{ duration: `${RAMPA}s`, target: OUVINTES }, { duration: `${ANTES + DURANTE}s`, target: OUVINTES }],
    },
    uploads: {
      executor: 'per-vu-iterations', vus: UPLOADS, iterations: 1, exec: 'enviar',
      startTime: `${RAMPA + ANTES}s`, maxDuration: '60s',
    },
  },
  thresholds: {
    'propagacao_ms{fase:antes}': ['p(95)<200'],
    'propagacao_ms{fase:durante}': ['p(95)<240'], // 200 ms + 20%
    upload_resposta_ms: ['p(95)<500'],
    upload_aceito: ['rate>0.99'],
    ws_erros: ['rate<0.01'],
  },
};

export function setup() {
  const est = createStation();
  const inicio = Date.now();
  const iniciaUploads = inicio + (RAMPA + ANTES) * 1000;
  const prazo = inicio + (RAMPA + ANTES + DURANTE) * 1000 + 5000;
  console.log(`LOAD-03: estação ${est.stationId}, ${OUVINTES} ouvintes, ${UPLOADS} uploads após ${RAMPA + ANTES}s`);
  return { ...est, iniciaUploads, prazo };
}

export function dj(d) { runDJ(d.stationId, d.djToken, SONDA_MS, d.prazo); }

export function ouvinte(d) {
  runListener(d.stationId, d.prazo, (agora, estado) => {
    // só conta depois que todos os ouvintes entraram (fim da rampa)
    if (agora < d.iniciaUploads - ANTES * 1000) return;
    const fase = agora < d.iniciaUploads ? 'antes' : 'durante';
    propagacao.add(agora - estado.startedAt, { fase });
    recebidos.add(1);
    erros.add(0);
  }, () => erros.add(1));
}

export function enviar(d) {
  const res = http.post(
    `${BASE_URL}/api/v1/stations/${d.stationId}/upload`,
    { file: http.file(arquivo, 'alarm.mp3', 'audio/mpeg') },
    { headers: { Authorization: `Bearer ${d.djToken}` }, tags: { name: 'upload' } },
  );
  upload.add(res.timings.duration);
  uploadAceito.add(res.status === 202);
  check(res, { 'upload respondeu 202': (r) => r.status === 202 });
}
