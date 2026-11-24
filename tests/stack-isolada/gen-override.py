#!/usr/bin/env python3
"""Gera o override do Compose que isola a stack de qualquer outra em execução.

Uso: docker compose config --format json | gen-override.py <projeto> > override.yml

O que o override faz (e por quê):
  * remove `container_name` e as portas publicadas: os nomes e as portas fixas
    (80, 8080, 3000, 5432...) colidem com a stack normal;
  * tira só o rótulo `traefik.enable=true` dos serviços roteados. O Traefik da
    stack normal usa exposedbydefault=false, então deixa de enxergá-los e não
    mistura as rotas. Todos os outros rótulos (rotas, middlewares, limites de
    taxa) ficam intactos, então o gateway do projeto isolado usa as regras reais;
  * o gateway isolado liga exposedbydefault=true e se restringe ao próprio
    projeto, para só enxergar os containers desta stack;
  * publica só o que o teste precisa, em portas altas: gateway, Prometheus,
    Grafana (com acesso anônimo, para captura de tela) e Jaeger.
"""
import json
import sys

projeto = sys.argv[1] if len(sys.argv) > 1 else "iso"
cfg = json.load(sys.stdin)
servicos = cfg["services"]

PUBLICAR = {
    "gateway": ["18080:80"],
    "prometheus": ["19090:9090"],
    "grafana": ["13000:3000"],
    "jaeger": ["16686:16686"],
}


def y(v):
    return json.dumps(v, ensure_ascii=False)  # JSON é YAML válido


out = ["# GERADO por tests/stack-isolada/gen-override.py. Não edite.", "services:"]
for nome, s in servicos.items():
    out.append(f"  {nome}:")
    if "container_name" in s:
        out.append("    container_name: !reset null")
    portas = PUBLICAR.get(nome)
    if portas:
        out.append("    ports: !override")
        out.extend(f"      - {y(p)}" for p in portas)
    elif s.get("ports"):
        out.append("    ports: !reset []")

    rotulos = dict(s.get("labels") or {})
    if rotulos.pop("traefik.enable", None) is not None:
        out.append("    labels: !override")
        out.extend(f"      {y(k)}: {y(v)}" for k, v in rotulos.items())

    if nome == "gateway":
        cmd = [c for c in s.get("command", []) if not c.startswith("--providers.docker.exposedbydefault")]
        cmd += [
            "--providers.docker.exposedbydefault=true",
            f"--providers.docker.constraints=Label(`com.docker.compose.project`,`{projeto}`)",
        ]
        out.append("    command: !override")
        out.extend(f"      - {y(c)}" for c in cmd)
    if nome == "grafana":
        env = ["GF_AUTH_ANONYMOUS_ENABLED=true", "GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer", "GF_AUTH_DISABLE_LOGIN_FORM=false"]
        out.append("    environment:")
        out.extend(f"      - {y(e)}" for e in env)

print("\n".join(out))
