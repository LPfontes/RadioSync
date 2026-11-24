// LOAD-04 — Rajada de POST /stations acima do limite do gateway.
// Métrica: respostas 429. Meta: o gateway bloqueia o excedente (ADR-0005:
// 10 req/min por IP, rajada de 5) e nenhuma requisição vira erro 5xx.
//
// Atenção: esgota a cota de criação de estação por ~1 min. Rode por último ou
// espere antes do próximo teste.
//
// Uso: tests/load/run.sh load-04 [-e TAXA=50 -e DURACAO=30]
import http from 'k6/http';
import { Counter, Rate } from 'k6/metrics';
import { BASE_URL, JSON_HEADERS } from './lib/common.js';

const TAXA = parseInt(__ENV.TAXA || '50'); // requisições por segundo
const DURACAO = parseInt(__ENV.DURACAO || '30');

const criadas = new Counter('estacoes_criadas');
const bloqueadas = new Counter('respostas_429');
const limitadas = new Rate('taxa_429');
const erros5xx = new Rate('taxa_5xx');

export const options = {
  scenarios: {
    rajada: {
      executor: 'constant-arrival-rate', rate: TAXA, timeUnit: '1s', duration: `${DURACAO}s`,
      preAllocatedVUs: 50, maxVUs: 200,
    },
  },
  thresholds: {
    taxa_429: ['rate>0.9'], // o grosso da rajada precisa ser barrado
    taxa_5xx: ['rate==0'], // bloquear não pode derrubar o serviço
    estacoes_criadas: ['count<=30'], // não deixa passar muito além do limite
  },
};

export default function () {
  const res = http.post(`${BASE_URL}/api/v1/stations`, '{}', {
    headers: JSON_HEADERS, tags: { name: 'criar-estacao' }, responseCallback: http.expectedStatuses(201, 429),
  });
  if (res.status === 201) criadas.add(1);
  if (res.status === 429) bloqueadas.add(1);
  limitadas.add(res.status === 429);
  erros5xx.add(res.status >= 500);
}
