# ADR-0003: Migração do monolito para serviços com Strangler Fig

- **Status:** Aceito
- **Data:** 2026-11-03
- **Responsável:** Membro 4 (Dados / Station Service)
- **Áreas:** Confiabilidade, Deployment

## Contexto

O RadioSync já funciona como um monolito Go (API REST + WebSocket + mídia +
arquivos estáticos) e está em uso. Reescrever tudo do zero até 11/12 é
arriscado: a equipe ficaria semanas sem uma versão funcional para demonstrar.

## Decisão

Evoluir **incrementalmente** com o padrão **Strangler Fig**:

1. Colocar o API Gateway (Traefik, ADR-0005) na frente do monolito. Todo o
   tráfego passa por ele desde já — o cliente não muda.
2. Extrair um serviço por vez e mudar só a regra de roteamento no gateway:

| Ordem | Rotas | Novo destino |
|---|---|---|
| 1 | `POST /api/v1/stations/{id}/upload`, `/youtube` | Media Service + Worker (ADR-0002) |
| 2 | `/ws/stations/*` | Sync Service com N réplicas (ADR-0001) |
| 3 | `/api/v1/stations*`, `/api/v1/library` | Station Service + Postgres (ADR-0006) |
| 4 | `/musicas/*` | MinIO / CDN (ADR-0006) |

3. O monolito continua respondendo às rotas ainda não migradas. Ao final, ele
   fica reduzido a servir o frontend (ou é desligado).

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Strangler Fig** (escolhida) | Sempre há uma versão funcional; rollback é trocar uma rota | Período de convivência com dois modelos de dados |
| Big-bang rewrite | Arquitetura "limpa" desde o início | Alto risco de não terminar; nada demonstrável no meio do caminho |
| Manter monolito modular | Menos infra | Não atende ao requisito de sistema distribuído da disciplina |

## Consequências

- **Positivas:** cada extração é pequena, testável e pode ser apresentada.
- **Negativas / trade-offs:** durante a transição, dados podem existir em dois
  lugares (`stations.json` e Postgres) — é preciso um script de migração único
  e um "dono" claro de cada dado em cada fase.
- **Como vamos validar:** os testes de integração de cada rota devem passar
  igual antes e depois da troca de destino no gateway.
