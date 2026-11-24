// Utilitários comuns dos testes de carga (k6).
//
// Os scripts rodam dentro da rede do Compose (veja run.sh), então o relógio do
// k6 e o dos servidores é o mesmo (mesmo kernel). Isso permite medir a latência
// de propagação como Date.now() - state.startedAt, sem erro de sincronização de
// relógio entre máquinas.
import http from 'k6/http';
import { sleep } from 'k6';
import { WebSocket } from 'k6/websockets';

export const BASE_URL = __ENV.BASE_URL || 'http://gateway';
// WS_BASE_URL permite apontar o WebSocket direto para o Sync Service (sem gateway).
export const WS_URL = __ENV.WS_BASE_URL || BASE_URL.replace(/^http/, 'ws');
export const JSON_HEADERS = { 'Content-Type': 'application/json' };

// Cria uma estação e devolve { stationId, djToken }. O gateway limita a criação
// (ADR-0005), então tenta de novo com espera se receber 429.
export function createStation() {
  for (let i = 0; i < 12; i++) {
    const res = http.post(`${BASE_URL}/api/v1/stations`, '{}', { headers: JSON_HEADERS });
    if (res.status === 201 || res.status === 200) {
      return res.json();
    }
    if (res.status !== 429) {
      throw new Error(`criar estação falhou: HTTP ${res.status} ${res.body}`);
    }
    sleep(10);
  }
  throw new Error('criar estação: o gateway respondeu 429 em todas as tentativas');
}

// Abre o WebSocket de uma estação. Com djToken, entra como DJ (o token vai no
// subprotocolo, como o navegador faz — ADR-0005).
export function openSocket(stationId, djToken) {
  const protocols = ['radiosync.v1'];
  if (djToken) protocols.push(`radiosync.auth.${djToken}`);
  return new WebSocket(`${WS_URL}/ws/stations/${stationId}`, protocols);
}

// DJ que dispara PLAY a cada intervaloMs até o prazo. Cada PLAY reinicia
// startedAt no servidor e é transmitido a todos os ouvintes: é a "sonda" cuja
// propagação os ouvintes medem.
export function runDJ(stationId, djToken, intervaloMs, prazoMs) {
  const ws = openSocket(stationId, djToken);
  ws.onopen = () => {
    ws.send(JSON.stringify({ type: 'PLAY' }));
    const timer = setInterval(() => {
      if (Date.now() >= prazoMs) {
        clearInterval(timer);
        ws.close();
        return;
      }
      ws.send(JSON.stringify({ type: 'PLAY' }));
    }, intervaloMs);
  };
  ws.onerror = (e) => console.error(`DJ: erro no WebSocket: ${e.error ? e.error() : e}`);
}

// Ouvinte que mede quanto o STATE_UPDATE demorou da decisão do servidor
// (startedAt) até chegar aqui. O primeiro STATE_UPDATE é o estado inicial da
// conexão, não uma propagação, e é ignorado.
export function runListener(stationId, prazoMs, aoReceber, aoErro) {
  const ws = openSocket(stationId);
  let inicial = true;
  ws.onopen = () => {
    setTimeout(() => ws.close(), Math.max(prazoMs - Date.now(), 100));
  };
  ws.onmessage = (ev) => {
    const agora = Date.now();
    let msg;
    try { msg = JSON.parse(ev.data); } catch (_) { return; }
    if (msg.type !== 'STATE_UPDATE' || !msg.state) return;
    if (inicial) { inicial = false; return; }
    if (msg.state.isPlaying) aoReceber(agora, msg.state);
  };
  ws.onerror = (e) => { if (aoErro) aoErro(e); };
  return ws;
}
