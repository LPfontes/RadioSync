# ADR-0001: Redis Pub/Sub para fan-out de eventos WebSocket entre réplicas

- **Status:** Aceito (implementação na Entrega 3)
- **Data:** 2026-11-03
- **Responsável:** Membro 1 (Sync Service)
- **Áreas:** Escalabilidade, Confiabilidade

## Contexto

Hoje cada estação tem um `ws.Hub` em memória dentro do único processo Go (`backend/internal/ws/hub.go`). O DJ e todos os ouvintes precisam estar conectados **no mesmo processo** para receber `PLAY`, `PAUSE`, `SEEK`, `STATE_UPDATE` etc.

Se colocarmos duas ou mais réplicas do servidor atrás de um load balancer, um ouvinte conectado na réplica B não recebe o `PLAY` que o DJ enviou para a réplica A. Ou seja: o sistema atual **não escala horizontalmente**.

Também há estado da estação (`PlaybackState`, playlist) guardado só em memória, o que impede qualquer réplica de atender qualquer cliente.

## Decisão

1. Cada réplica do Sync Service publica os eventos de uma estação no canal Redis `station:{id}:events` e assina esse mesmo canal. Ao receber uma mensagem do Redis, a réplica a repassa apenas para os clientes WebSocket **locais** daquela estação (Messaging Fan-out).
2. O `PlaybackState` e a playlist passam a ser guardados em Redis (`station:{id}:state`, `station:{id}:playlist`), tornando as réplicas *stateless* — o estado deixa de morar no processo.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Redis Pub/Sub** (escolhida) | Latência sub-milissegundo; simples; o mesmo Redis guarda o estado | Entrega *at-most-once*: mensagem perdida se a réplica estiver desconectada |
| Sticky sessions (todos da estação na mesma réplica) | Nenhuma mudança no código | Estação grande não cabe numa réplica; queda da réplica derruba a estação inteira |
| NATS / Kafka | Garantias de entrega maiores; replay | Mais um componente para operar; replay é desnecessário para eventos de player |
| RabbitMQ fanout exchange | Já teremos RabbitMQ (ADR-0002) | Latência maior e mais overhead por mensagem que Redis |

## Consequências

- **Positivas:** o Sync Service pode ter N réplicas; uma réplica pode cair sem derrubar a estação (os clientes reconectam em outra).
- **Negativas / trade-offs:** Pub/Sub é *fire-and-forget*. Mitigação: o cliente já envia `SYNC_REQUEST` ao reconectar e o estado completo é lido do Redis, então um evento perdido é corrigido no próximo `STATE_UPDATE`. Redis vira ponto único de falha (mitigável com Redis Sentinel, fora do escopo).
- **Como vamos validar:** teste de integração com 2 réplicas (DJ na réplica A, ouvinte na B); teste de carga com k6 medindo p95 da latência de propagação do `PLAY` com 500 ouvintes; teste de resiliência derrubando uma réplica.
