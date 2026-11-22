# ADR-0008: Pipeline DevSecOps no CI (SCA, SAST, segredos, imagem e SBOM)

- **Status:** Aceito
- **Data:** 2026-11-22
- **Responsável:** Membro 3 (Infra / DevOps / Segurança)
- **Áreas:** Segurança, Deployment

## Contexto

O CI da Entrega 2 (`ci.yml`) testava e compilava, mas não olhava para
segurança. Rodar as ferramentas uma vez à mão mostrou o tamanho do problema:

| Ferramenta | O que achou |
|---|---|
| `gosec` (SAST) | **Injeção de argumentos no yt-dlp** que podia levar a execução de comandos no servidor (corrigida em PR próprio), servidor HTTP sem timeouts (Slowloris), `cookies.txt` com permissão `0644`, além de ~40 avisos médios ou falsos positivos |
| `npm audit` (SCA) | 7 vulnerabilidades em dependências de produção, 6 *high* (axios, vue, follow-redirects…) |
| Trivy (imagem) | 3 *HIGH* em pacotes do sistema (musl, zlib), todos com correção |
| Trivy (config) | Container rodando como `root` |
| gitleaks | 82 segredos no histórico: cookies de sessão do YouTube em `backend/data/cookies.txt` (commit de 22/07) |
| `govulncheck` (SCA Go) | **21 vulnerabilidades da biblioteca padrão alcançáveis pelo código** (ex.: `crypto/x509` via `ListenAndServe`), porque o `go.mod` fixava `go 1.26.1` e o CI usava exatamente essa versão. Localmente não apareceu porque a máquina tinha o Go 1.27; só o primeiro run do pipeline mostrou. |

Sem um pipeline, cada um desses problemas volta no próximo PR sem ninguém ver.

## Decisão

Criar o workflow `.github/workflows/security.yml`, que roda em todo PR, em push
para `dev`/`main`, toda segunda-feira (CVEs novas aparecem sem mudança no
código) e sob demanda.

**Bloqueiam o merge (gates):**

| Job | Ferramenta | Regra |
|---|---|---|
| `go-deps` | govulncheck | qualquer vulnerabilidade alcançável pelo código |
| `npm-deps` | npm audit | *high* ou pior |
| `secrets` | gitleaks, histórico inteiro | qualquer segredo fora do `.gitleaksignore` |
| `container` | Trivy na imagem | *HIGH*/*CRITICAL* com correção disponível |

**Só reportam (aba Security → Code scanning, em SARIF):** gosec (SAST) e Trivy
config (Dockerfile e compose). **Artefato:** SBOM SPDX da imagem (syft).

**Cadeia de suprimentos do próprio pipeline:** actions fixadas por SHA de
commit (uma tag pode ser movida; um SHA não), ferramentas fixadas por versão e
o binário do gitleaks verificado por checksum.

Correções feitas junto, para o pipeline nascer verde:
- `toolchain go1.26.11` no `go.mod` (último patch da 1.26; a versão da linguagem continua 1.26), o que zera o govulncheck;
- `npm audit fix` (só o `package-lock.json` mudou; versões compatíveis);
- `apk upgrade` na imagem final do Dockerfile;
- `ReadHeaderTimeout` (10 s), `IdleTimeout` (120 s) e `MaxHeaderBytes` no servidor HTTP;
- `.gitleaksignore` com as 2 impressões digitais (82 achados) do `cookies.txt` antigo.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Workflow próprio com CLIs** (escolhida) | Ferramentas abertas, versão fixada, roda igual na máquina local | Mais YAML para manter |
| GitHub Advanced Security / CodeQL | Integrado ao GitHub | CodeQL não cobre a imagem nem o histórico de segredos; GHAS é pago em repositório privado |
| Snyk / SonarCloud | Painel pronto | Conta externa, token no repositório, limite do plano gratuito |
| Dependabot só | Zero configuração | Só dependências; não acha bug no código, segredo nem CVE da imagem |
| Tudo bloqueando desde o início | Mais rígido | O gosec tem ~40 avisos, muitos falsos positivos: o time pararia de olhar ou começaria a ignorar em massa |

## Consequências

- **Positivas:** SCA, SAST, segredos, scan de container e SBOM em todo PR
  (DevSecOps e Supply Chain Security, valorizados no documento da disciplina).
  O primeiro uso já pegou uma falha crítica.
- **Negativas / trade-offs:**
  - O gosec e o Trivy config **não bloqueiam** ainda. Plano: triar os avisos
    no Code scanning (corrigir ou marcar `#nosec` com justificativa) e depois
    passar o gosec a bloquear *HIGH*.
  - **Container como `root`:** corrigir muda a permissão dos volumes
    `./musicas` e `./data` no servidor; fica para a estratégia de deployment
    (ADR-0007).
  - O `yt-dlp` e os pacotes `pip` da imagem são instalados na última versão,
    sem checksum, porque o yt-dlp precisa acompanhar as mudanças do YouTube.
    O Trivy cobre os pacotes Python; o binário do yt-dlp, não.
  - Os segredos do `.gitleaksignore` continuam no histórico público. A
    baseline não os torna seguros: a sessão do Google que gerou os cookies
    precisa ser encerrada.
  - Cerca de 3 a 4 minutos a mais por PR (o job da imagem é o mais lento).
- **Como vamos validar:** o próprio workflow verde no PR que o introduz; um
  token falso de teste num arquivo foi detectado pelo gitleaks; com o
  `apk upgrade`, o Trivy da imagem dá 0 *HIGH*/*CRITICAL* corrigíveis; e uma
  conexão que nunca termina o cabeçalho é encerrada pelo servidor em 10,0 s.
