# ADR-0006: Persistência poliglota — PostgreSQL, Redis e MinIO

- **Status:** Proposto
- **Data:** 2026-11-03
- **Responsável:** Membro 4 (Dados / Station Service)
- **Áreas:** Escalabilidade, Confiabilidade, Desempenho

## Contexto

Hoje a persistência é:

- `stations.json` (e catálogos JSON) gravados por `handler/persist.go`, com
  escrita atômica (arquivo temporário + rename) e salvamento periódico.
- Arquivos `.opus` numa pasta local (`MUSIC_DIR`), servidos pelo próprio Go.

Isso funciona com **uma** instância. Com várias réplicas, cada uma teria seu
próprio `stations.json` e sua própria pasta de músicas — os dados divergem.

## Decisão

Cada tipo de dado vai para o armazenamento mais adequado (**Polyglot Persistence**):

| Dado | Armazenamento | Motivo |
|---|---|---|
| Estado "quente" do player e playlist | **Redis** | Leitura/escrita muito frequente, baixa latência (ADR-0001) |
| Estações, catálogo de faixas, metadados | **PostgreSQL** | Dados duráveis e relacionais; consultas da biblioteca global |
| Arquivos `.opus` e originais | **MinIO** (compatível com S3) | Compartilhado entre réplicas e workers; URLs pré-assinadas |

Cada serviço é dono do seu banco (**Database-per-service**): o Station Service
é o único que escreve no Postgres; o Media Worker é o único que escreve no MinIO.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Postgres + Redis + MinIO** (escolhida) | Cada dado no lugar certo; tudo roda no Compose | Três componentes para operar |
| Manter JSON em volume compartilhado | Nenhuma mudança | Escrita concorrente de várias réplicas corrompe o arquivo |
| Só Redis (com AOF) | Um componente a menos | Ruim para consultas e para dados que precisam durar |
| MongoDB | Esquema flexível | Dados são relacionais (estação ↔ faixas); equipe conhece SQL |

## Consequências

- **Positivas:** réplicas e workers compartilham os mesmos dados; backups e
  consultas ficam simples.
- **Negativas / trade-offs:** consistência eventual entre Redis (estado quente)
  e Postgres (registro durável); é preciso um script de migração do
  `stations.json` atual.
- **Como vamos validar:** teste de integração que cria estação na réplica A e
  lê na réplica B; teste de resiliência reiniciando o Postgres.
