# ADR-0005: API Gateway (Traefik) com autenticação de admin e rate limiting

- **Status:** Aceito (gateway na Entrega 2; auth de admin implementada na Entrega 3)
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
2. No gateway: **rate limiting** global (50 req/s, burst 100) e, na Entrega 3,
   limites por rota, por IP, nas rotas caras ou abusáveis:

   | Rota | Limite | Por quê |
   |---|---|---|
   | `POST /api/v1/stations` | 10/min, burst 5 | Criação em massa de salas |
   | `POST .../upload` e `.../youtube` | 6/min, burst 3; no máximo 2 em andamento; corpo até 100 MB (`413`) | CPU do FFmpeg/yt-dlp e disco |
   | `POST .../claim-dj` | 10/min, burst 5 | Tomar a bancada em loop |
   | Demais rotas | global | Leitura e WebSocket não podem ser travados |

   O `429` vem com `Retry-After`, e o frontend mostra "tente de novo em N s".
   Medido na stack, pelo gateway: 12 `POST /stations` seguidos dão 5 `201` e
   7 `429` (LOAD-04); 8 `claim-dj` dão 5 e 3; 6 uploads dão 3 e 3; o 3º upload
   simultâneo do mesmo IP recebe `429` na hora; 101 MB recebem `413`; 30 `GET`
   da estação passam todos.
   **Trade-off:** a porta 8080 do app continua publicada e não passa pelo
   gateway, então quem a acessa direto não tem esses limites (a mesma rajada
   deu 12 `201`). Em produção a 8080 deve ficar só em `127.0.0.1` ou fora da
   rede pública; isso entra na estratégia de deployment (ADR-0007).
3. Proteger `/api/v1/admin/*` com um JWT de papel `admin` (middleware no Go,
   validado também no gateway via ForwardAuth) e **remover o `djToken`** da
   resposta de listagem.
4. Restringir CORS às origens do frontend.
5. Tirar segredos do repositório: `git rm --cached backend/.env backend/data/cookies.txt`,
   **trocar o `JWT_SECRET`** e regenerar os cookies do YouTube (os atuais devem
   ser considerados vazados).

## Implementação (Entrega 3): token de DJ fora da URL

O WebSocket recebia o token de DJ na query string (`?role=dj&token=...`), e a
URL completa ia para o log do chi no Go e para o access log do Traefik. Quem
lesse os logs virava DJ de qualquer sala.

- **WebSocket:** o token vai no header `Sec-WebSocket-Protocol`, como
  subprotocolo (`new WebSocket(url, ["radiosync.v1", "radiosync.auth.<token>"])`).
  É o único header que o navegador deixa definir no WebSocket. O servidor
  responde só com `radiosync.v1`, sem ecoar o token, e ignora `?token=` de
  clientes antigos (registra um aviso sem o token).
- **`GET /stations/{id}`:** aceita o token só em `Authorization: Bearer`; a
  alternativa por query string foi removida (o frontend já usava o header).
- **Links `/radio/ID?token=...`:** continuam funcionando, mas o token sai da
  barra de endereço na hora. O primeiro acesso ainda passa pelo log, então o
  app não deve gerar esse tipo de link.

Alternativa descartada: autenticar com a primeira mensagem depois de conectar.
Exigiria trocar o papel do cliente já registrado no Hub, que é a frente do
Sync Service (Membro 1).

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Traefik** (escolhida) | Configuração por labels; dashboard; métricas Prometheus nativas | Menos conhecido pela equipe que Nginx |
| Nginx | Muito difundido | Configuração estática; recarregar ao escalar réplicas |
| Kong | Plugins de auth prontos | Precisa de banco próprio; pesado para o projeto |
| Sem gateway (só middlewares no Go) | Menos infra | Cada serviço novo reimplementa auth/limite; não há ponto para o Strangler Fig |

## Implementação (Entrega 3): token de DJ e JWT_SECRET

Problemas encontrados no código: o token de DJ não tinha `exp` (um token
vazado valia para sempre) e, sem `JWT_SECRET`, o servidor assinava com o valor
fixo `dev-secret-change-in-production`, que está no repositório público. Com
ele qualquer pessoa forja um token de DJ para qualquer estação.

- **`JWT_SECRET` validado na subida:** valores de exemplo conhecidos ou com
  menos de 32 caracteres fazem o servidor abortar com uma mensagem dizendo
  como gerar um. Sem a variável, o servidor usa um segredo aleatório gerado no
  processo (nunca um valor fixo) e avisa no log que os tokens caem a cada
  reinício. Isso mantém o `go run` local funcionando sem `.env`.
- **Token de DJ com expiração:** `exp` com `DJ_TOKEN_TTL` (padrão 24h), e a
  validação passa a exigir `exp` e HS256. Tokens antigos, sem `exp`, deixam de
  valer: o DJ cai para ouvinte e pode assumir a bancada de novo.
- **Expiração deslizante:** `GET /stations/{id}` devolve um token novo para
  quem já é DJ (com `Cache-Control: no-store`), e o frontend salva esse token.
  Na prática só expira quem fica `DJ_TOKEN_TTL` sem abrir a sala.

Pendente: o WebSocket recebe o token na query string (`?token=`), que aparece
nos logs de acesso do Go e do Traefik. A expiração limita o estrago, mas o
ideal é tirar o token da URL.

## Consequências

- **Positivas:** um único lugar para segurança de borda, roteamento e métricas.
- **Negativas / trade-offs:** o gateway é um ponto único de falha e adiciona
  um salto de rede (~1 ms).
- **Como vamos validar:** testes de integração garantindo `401` nas rotas
  admin sem token; teste de carga confirmando `429 Too Many Requests` acima do limite.

## Implementação (Entrega 3): CORS e Origin do WebSocket

- **Regra:** só a mesma origem por padrão. Origens extras entram por
  `ALLOWED_ORIGINS` (lista separada por vírgula, sem curinga). O app é servido
  na mesma origem da API (pelo Go, pelo gateway e pelo proxy do Vite, que
  preservam o `Host`), então em geral a lista fica vazia.
- **CORS:** `AllowOriginFunc` com a allowlist (com `AllowedOrigins` vazio o
  `rs/cors` liberaria todas as origens), sem `AllowCredentials` (a autenticação
  é por header `Bearer`, não por cookie) e só os headers `Authorization` e
  `Content-Type`.
- **WebSocket:** o `CheckOrigin` deixou de aceitar tudo. Aceita requisições sem
  `Origin` (clientes que não são navegador, como k6), a mesma origem e a
  allowlist; o resto recebe `403`, o que bloqueia Cross-Site WebSocket
  Hijacking (uma página de outro site abrindo a conexão no navegador da vítima).
- **Código:** `internal/security` concentra as regras, com testes de
  normalização, CORS e handshake real do WebSocket.

Validado na stack: `Origin: https://evil.example` recebe `403` no WebSocket e
nenhum cabeçalho CORS, pelo gateway, pela porta 8080 e pelo proxy do Vite;
mesma origem e clientes sem `Origin` recebem `101`; com
`ALLOWED_ORIGINS=https://radio.exemplo.com`, essa origem passa no preflight e
no WebSocket.

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

## Implementação (Entrega 3): autenticação do admin

- **Login:** `POST /api/v1/admin/login` troca a senha (`ADMIN_PASSWORD`,
  comparada em tempo constante) por um JWT HS256 com `role=admin` e `exp`
  (`ADMIN_TOKEN_TTL`, padrão 8h). Sem `ADMIN_PASSWORD`, as rotas de admin
  respondem `503`: seguro por padrão.
- **Middleware no Go (`RequireAdmin`):** protege `/api/v1/admin/*` e
  `/api/v1/debug`. Exige `exp` e `role=admin`, então um token de DJ não vale
  como admin e o token de admin não vale como DJ.
- **ForwardAuth no Traefik:** um router dedicado para as rotas de admin e
  `/debug` consulta `/api/v1/admin/verify` antes de repassar. O Go valida de
  novo, o que mantém a proteção para quem acessa a porta 8080 direto
  (defesa em profundidade) e deixa o gateway pronto para quando o admin virar
  um serviço separado (Strangler Fig).
- **Força bruta:** o login tem rate limit próprio no gateway (5/min, burst 5).
- **Vazamento do `djToken`:** a listagem não devolve mais o token. O painel
  pede um token de DJ por estação, sob demanda, em
  `POST /api/v1/admin/stations/{id}/dj-token` (registrado em log).
- **Frontend:** o painel `/admin` tem tela de login; o token fica no
  `sessionStorage` e um `401` devolve para o login.

Validado na stack do compose, pelo gateway e pela porta 8080: sem token `401`,
token de DJ `401`, token de admin `200`, listagem sem `djToken`, e, esgotado
o burst de 5 logins no minuto, as tentativas seguintes pelo gateway recebem `429`. Testes em
`internal/auth/admin_test.go` e `internal/handler/adminauth_test.go`.
