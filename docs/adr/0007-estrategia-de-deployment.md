# ADR-0007: Estratégia de deployment

- **Status:** Aceito
- **Data:** 2026-11-22
- **Responsável:** Membro 3 (Infra / DevOps / Segurança)
- **Áreas:** Deployment, Confiabilidade

## Contexto

Até aqui, publicar uma versão era `docker compose up -d --build` com a tag
`latest`: não havia como saber qual versão estava no ar, nem como voltar para a
anterior, e nada verificava se a versão nova tinha subido de fato.

O que restringe a escolha hoje:

1. **O app é um monolito com estado em memória.** Estações, playlists e o Hub
   de WebSocket vivem na memória do processo, com cópia em `stations.json` no
   disco. Duas instâncias ao mesmo tempo teriam estados divergentes e
   escreveriam no mesmo arquivo.
2. **Um único host** com Docker Compose, atrás do Traefik.
3. **O áudio não depende de conexão contínua.** Pelo ADR-0004, cada ouvinte
   baixa o `.opus` e calcula a posição pelo timestamp. Se o WebSocket cair, ao
   reconectar ele pede `SYNC` e volta ao ponto certo. Uma interrupção curta do
   servidor não para a música de quem já está ouvindo.
4. Time pequeno, prazo curto: o deploy tem que ser simples de rodar e de
   desfazer.

## Como escolhemos (critérios)

| Critério | Peso aqui | Por quê |
|---|---|---|
| Estado e compatibilidade de dados | **decisivo** | Estado em memória impede duas versões ao mesmo tempo |
| Tempo para voltar atrás | alto | Uma versão ruim tem que sair do ar sozinha |
| Indisponibilidade tolerada | médio | Ouvintes toleram segundos (item 3); minutos não |
| Custo de infraestrutura | médio | Um host; dobrar a infra só para deploy não compensa |
| Complexidade para o time | médio | Precisa caber em um script que qualquer membro rode |

## Alternativas consideradas

| Estratégia | Serve hoje? | Motivo |
|---|---|---|
| **Big bang / recreate in-place** (`up --build` com `latest`) | não | Sem versão identificável, sem verificação e sem rollback |
| **Recreate imutável com health gate e rollback** | **sim (escolhida)** | Uma instância por vez respeita o estado em memória; a tag por SHA permite voltar; o `/healthz` decide |
| **Rolling** (réplicas trocadas aos poucos) | ainda não | Exige réplicas sem estado; hoje cada réplica teria o próprio estado |
| **Blue-green** (duas versões no ar, troca de tráfego) | ainda não | As duas instâncias divergiriam no estado e disputariam o `stations.json` |
| **Canary** (fração do tráfego na versão nova) | ainda não | Mesmo problema do blue-green, e ouvintes da mesma sala ficariam em versões diferentes |
| **Shadow / dark launch** | não | Duplicar comandos de DJ em uma instância sombra causaria efeitos colaterais (downloads, gravação de estado) |
| **Feature flags** | complementar | Úteis para ligar funcionalidades sem deploy, mas não substituem a estratégia de deploy |

Também decidimos: **imutável em vez de in-place** (a imagem não muda depois de
construída; a versão é o SHA do commit) e **agentless** (o deploy é um script
rodado no host, sem agente instalado).

## Decisão

**Agora (monolito): recreate imutável com health gate, falha rápida e rollback
automático,** em `scripts/deploy.sh`:

1. Versão = SHA curto do commit; a imagem `lpfonts/radiosync:<sha>` é
   construída (ou baixada do registry). O script recusa construir com mudanças
   não commitadas, para a tag sempre corresponder ao código.
2. Recria só o container do app (`docker compose up --no-deps`). O antigo
   recebe `SIGTERM` e **salva o estado antes de sair**; o novo o carrega.
3. Espera `/healthz` responder 200 **com a versão nova** e o `HEALTHCHECK` do
   Docker ficar `healthy`. O Traefik só roteia para containers saudáveis, então
   esse é o momento em que o usuário volta a ser atendido.
4. **Falha rápida:** se o container reinicia, encerra ou fica `unhealthy`, o
   rollback começa na hora, sem esperar o timeout.
5. **Rollback** para a versão anterior (anotada antes da troca), também com
   health gate. Tudo fica registrado em `deploy/history.log`.

Peças de suporte:
- `GET /healthz` com status, versão, número de estações e uptime.
- `HEALTHCHECK` na imagem com `--start-interval=1s` (sem isso o Traefik só
  mandava tráfego ~10 s depois de o container subir).
- Salvamento final no `SIGTERM` e `stop_grace_period: 20s` no compose.

**Fluxo de branches até o deploy:** branch da issue → PR e merge em `dev` (com
1 aprovação e os workflows `CI` e `Security` verdes) → PR e merge de `dev` em
`main` → deploy do commit de `main` com `./scripts/deploy.sh`.

**Porta 8080:** publicada só em `127.0.0.1` por padrão (`HOST_PORT` no
`.env.example`). Exposta em todas as interfaces, ela contornava o gateway e,
com ele, o rate limit, a auth do admin e o CORS (ADR-0005).

**Depois, com Sync (Redis) e Station (Postgres + MinIO) sem estado:**
rolling com `--scale` e canary/blue-green por pesos no Traefik, por exemplo:

```yaml
http:
  services:
    sync:
      weighted:
        services:
          - name: sync-v1@docker
            weight: 90
          - name: sync-v2@docker   # canary: 10%, depois 50%, depois 100%
            weight: 10
```

Com o áudio servido pelo MinIO, a troca de versão também deixa de interromper
o download do `.opus`.

## Resultados (stack local, Docker 29 + Traefik 3.6)

Medidos com um "ouvinte" simulado: HTTP a cada 100 ms pelo gateway e um
WebSocket que reconecta a cada 3 s, como o frontend.

| Cenário | Resultado |
|---|---|
| Deploy de versão para versão | **1,5 s** até `healthy` (script); ouvinte: falhas HTTP numa janela de 1,9 s; WebSocket reconectou em **3,0 s** e recebeu `SYNC` |
| Estado alterado via WebSocket logo antes do deploy | preservado |
| `docker stop` no app | "sinal recebido: salvando o estado", saída em 0,1 s com código 0 |
| Versão que quebra na subida (`panic`) | rollback automático; **2,4 s** fora do ar do ponto de vista do ouvinte |
| Mesma versão quebrada, antes da falha rápida | ~22 s fora do ar (o script esperava o timeout de 20 s) |
| Tag inexistente | o script para antes de tocar na versão no ar |
| Antes do `--start-interval` | o script media 0,4 s, mas o gateway ficava ~10 s sem a versão nova |

## Consequências

- **Positivas:** toda versão no ar é identificável e reversível com um comando;
  uma versão quebrada sai do ar sozinha em segundos; o número que o script
  mostra é o que o usuário sente.
- **Negativas / trade-offs:**
  - **Há indisponibilidade** em todo deploy (~1,5 s), e uma versão ruim custa
    o tempo de detecção mais o rollback. Zero downtime só depois que o estado
    sair da memória (ver "Depois").
  - O **áudio é servido pelo próprio app**: numa troca de ~1,5 s o navegador
    normalmente já tem o trecho em buffer, mas isso **não foi medido**.
  - A falha rápida pega crash e `unhealthy`; um bug que mantém o `/healthz`
    verde passa. O `/healthz` checa só o essencial (estado carregado, diretórios
    acessíveis).
  - O deploy ainda é **manual** (script no servidor). Próximo passo: o CI
    publicar a imagem `:<sha>` no GHCR ao mergear em `main`, com SBOM e
    proveniência, e o servidor só rodar `./scripts/deploy.sh <sha>` (o script
    já tenta `docker pull`). Falta decidir o registry e configurar os segredos
    de acesso ao servidor.
  - **Container como `root`** (apontado pelo Trivy, ADR-0008) continua. Trocar
    exige ajustar o dono dos volumes `./musicas` e `./data` no servidor; fica
    para quando os volumes forem para o MinIO e o Postgres.
- **Como validar:** os cenários da tabela de resultados, repetidos em cada
  versão que mexer em deploy; o `deploy/history.log` registra cada troca e
  cada rollback.
