# Stack isolada: o Compose completo ao lado de outro que já esteja rodando

O `docker-compose.yml` usa `container_name` e portas fixas (80, 8080, 3000,
5432…) e o Traefik descobre qualquer container com rótulos `traefik.*`. Por isso
duas stacks na mesma máquina conflitam, e a segunda pode até **bagunçar as rotas
da primeira**. Esta pasta sobe a stack completa como um projeto separado (`iso`),
**sem tocar na stack normal**, reaproveitando os rótulos reais do Compose.

```bash
cp .env.example .env            # e defina o JWT_SECRET (e ADMIN_PASSWORD, se for testar o admin)
tests/stack-isolada/up.sh       # constrói e sobe
tests/stack-isolada/down.sh     # derruba e apaga os volumes dela
```

Portas no host: gateway **18080**, Prometheus **19090**, Grafana **13000**
(acesso anônimo de leitura), Jaeger **16686**. O resto só existe dentro da rede
`iso_default`.

Para rodar os testes contra ela, aponte os scripts para o projeto isolado:

```bash
export COMPOSE_PROJECT_NAME=iso
export COMPOSE_FILE=docker-compose.yml:tests/stack-isolada/override.yml
export IMAGE_TAG=iso COMPOSE_NETWORK=iso_default GATEWAY_LOCAL=http://localhost:18080
tests/load/run.sh load-04
tests/resilience/res-01-replica-sync.sh
```

## Como o isolamento funciona

`gen-override.py` lê o `docker compose config` e gera `override.yml`:

| Problema | O que o override faz |
|---|---|
| nomes de container fixos | remove `container_name` |
| portas fixas | remove as publicações (só publica as 4 acima, em portas altas) |
| o Traefik da stack normal enxergar os containers daqui | tira **só** o rótulo `traefik.enable=true`; ele usa `exposedbydefault=false` e passa a ignorá-los |
| o gateway isolado precisar das rotas | liga `exposedbydefault=true` e restringe ao projeto `iso` (`providers.docker.constraints`) |
| imagens `:latest` da stack normal | `IMAGE_TAG=iso` usa outra tag |

Todos os outros rótulos (rotas, middlewares, limites de taxa) ficam intactos,
então o gateway isolado aplica as **mesmas regras** do Compose real.

## Limites

- As duas stacks dividem CPU, memória e disco da mesma VM do Docker; em testes
  de carga isso adiciona ruído.
- O Traefik dos dois lados lê o mesmo `docker.sock`.
