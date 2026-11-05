# ADR-0005: API Gateway (Traefik) com autenticação de admin e rate limiting

- **Status:** Aceito (gateway já no docker-compose; regras de auth na Entrega 3)
- **Data:** 2026-11-03
- **Responsável:** Membro 3 (Infra / DevOps)
- **Áreas:** Segurança, Escalabilidade

## Contexto

Ao analisar o código atual encontramos problemas de segurança concretos:

1. As rotas `/api/v1/admin/*` **não exigem autenticação**. Em particular,
   `GET /api/v1/admin/stations` devolve o `djToken` de todas as estações —
   qualquer pessoa consegue virar DJ de qualquer sala.
2. Não há limite de requisições: `POST /api/v1/stations` e os endpoints de
   upload/YouTube podem ser abusados (criação massiva de salas, CPU do FFmpeg).
3. CORS está configurado com `AllowedOrigins: *` junto com `AllowCredentials: true`.
4. O repositório contém `backend/.env` (com o `JWT_SECRET`) e
   `backend/data/cookies.txt` (cookies de sessão do YouTube) commitados.

Além disso, a migração Strangler Fig (ADR-0003) precisa de um ponto único de
entrada para redirecionar rotas.

## Decisão

1. Usar **Traefik** como API Gateway e load balancer, com descoberta automática
   dos containers via labels do Docker.
2. No gateway: **rate limiting** (já configurado: 50 req/s, burst 100) e,
   na Entrega 3, um limite mais rígido para `POST /stations`, `/upload` e `/youtube`.
3. Proteger `/api/v1/admin/*` com um JWT de papel `admin` (middleware no Go,
   validado também no gateway via ForwardAuth) e **remover o `djToken`** da
   resposta de listagem.
4. Restringir CORS às origens do frontend.
5. Tirar segredos do repositório: `git rm --cached backend/.env backend/data/cookies.txt`,
   **trocar o `JWT_SECRET`** e regenerar os cookies do YouTube (os atuais devem
   ser considerados vazados).

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Traefik** (escolhida) | Configuração por labels; dashboard; métricas Prometheus nativas | Menos conhecido pela equipe que Nginx |
| Nginx | Muito difundido | Configuração estática; recarregar ao escalar réplicas |
| Kong | Plugins de auth prontos | Precisa de banco próprio; pesado para o projeto |
| Sem gateway (só middlewares no Go) | Menos infra | Cada serviço novo reimplementa auth/limite; não há ponto para o Strangler Fig |

## Consequências

- **Positivas:** um único lugar para segurança de borda, roteamento e métricas.
- **Negativas / trade-offs:** o gateway é um ponto único de falha e adiciona
  um salto de rede (~1 ms).
- **Como vamos validar:** testes de integração garantindo `401` nas rotas
  admin sem token; teste de carga confirmando `429 Too Many Requests` acima do limite.

## Validação (Entrega 2)

Subimos o `docker-compose.yml` completo localmente (Docker Engine 29) e
testamos cada serviço. Duas coisas não funcionavam e foram corrigidas:

- `traefik:v3.3` não consegue falar com o Docker Engine 29 (versão da API do
  Docker antiga demais). O provider ficava em erro e o gateway devolvia 404
  pra tudo. Subimos para `traefik:v3.6`.
- `minio/minio` não existe mais no Docker Hub, o pull falhava e o compose
  inteiro não subia. Trocamos por `cgr.dev/chainguard/minio`.

Rate limit testado com 1500 requisições concorrentes (~600 req/s) passando
pelo gateway: 221 respostas `200` e 1279 `429`. Bate com a configuração
(burst de 100 + 50 req/s durante ~2,5 s).

Também confirmamos o problema 1 do contexto: `GET /api/v1/admin/stations`
responde `200` sem nenhuma autenticação, inclusive passando pelo gateway.
A correção fica para a Entrega 3.
