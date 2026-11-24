// Sonda de resiliência: ouvintes que reconectam sozinhos, como o frontend, e
// medem quanto tempo levaram para voltar a receber o estado da estação.
//
// Não é um teste sozinho: os scripts res-*.sh sobem esta sonda em segundo plano
// e injetam a falha (docker stop/restart) no meio da execução.
//
// Variáveis (-e): OUVINTES=20 DURACAO=60 DJ=0|1
import { Trend, Counter } from 'k6/metrics';
import { createStation, openSocket, runDJ } from '../load/lib/common.js';

const OUVINTES = parseInt(__ENV.OUVINTES || '20');
const DURACAO = parseInt(__ENV.DURACAO || '60');
const COM_DJ = (__ENV.DJ || '0') === '1';
const ESPERA_RECONEXAO_MS = 500;

const reconexao = new Trend('reconexao_ms', true); // da queda até o 1º STATE_UPDATE da nova conexão
const quedas = new Counter('quedas_de_conexao');
const semVoltar = new Counter('ouvintes_sem_reconectar'); // ainda caídos no fim do teste
const ultimoUpdate = new Trend('ultimo_update_atras_ms', true); // com DJ: há quanto tempo não chega evento

export const options = {
  scenarios: {
    ouvintes: { executor: 'per-vu-iterations', vus: OUVINTES, iterations: 1, maxDuration: `${DURACAO + 60}s`, exec: 'ouvinte' },
    ...(COM_DJ ? { dj: { executor: 'per-vu-iterations', vus: 1, iterations: 1, maxDuration: `${DURACAO + 60}s`, exec: 'dj' } } : {}),
  },
  thresholds: {
    reconexao_ms: ['p(95)<5000'],
    ouvintes_sem_reconectar: ['count==0'],
    ...(COM_DJ ? { ultimo_update_atras_ms: ['p(95)<5000'] } : {}),
  },
};

export function setup() {
  const est = createStation();
  console.log(`sonda: estação ${est.stationId}, ${OUVINTES} ouvintes, ${DURACAO}s, DJ=${COM_DJ}`);
  return { ...est, prazo: Date.now() + DURACAO * 1000 };
}

export function dj(d) { runDJ(d.stationId, d.djToken, 2000, d.prazo); }

export function ouvinte(d) {
  const est = { caiuEm: 0, ultimo: 0 };
  conectar(d, est);
}

function conectar(d, est) {
  const ws = openSocket(d.stationId);
  ws.onopen = () => {
    setTimeout(() => ws.close(), Math.max(d.prazo - Date.now(), 100)); // encerra no prazo
  };
  ws.onmessage = (ev) => {
    let msg;
    try { msg = JSON.parse(ev.data); } catch (_) { return; }
    if (msg.type !== 'STATE_UPDATE') return;
    const agora = Date.now();
    est.ultimo = agora;
    if (est.caiuEm) { reconexao.add(agora - est.caiuEm); est.caiuEm = 0; }
  };
  ws.onclose = () => {
    if (Date.now() >= d.prazo) {
      if (est.caiuEm) semVoltar.add(1);
      if (COM_DJ && est.ultimo) ultimoUpdate.add(d.prazo - est.ultimo);
      return;
    }
    if (!est.caiuEm) { est.caiuEm = Date.now(); quedas.add(1); }
    setTimeout(() => conectar(d, est), ESPERA_RECONEXAO_MS);
  };
}
