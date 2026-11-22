# ADR-0001: Redis Pub/Sub para fan-out de eventos WebSocket entre réplicas

- **Status:** Implementado (Entrega 3)
- **Data:** 2026-11-03 (atualizado em 2026-11-22)
- **Responsável:** Membro 1 — Nathan (Sync Service)
- **Áreas:** Escalabilidade, Confiabilidade

## Contexto

Hoje cada estação tem um `ws.Hub` em memória dentro do único processo Go (`backend/internal/ws/hub.go`). O DJ e todos os ouvintes precisam estar conectados **no mesmo processo** para receber `PLAY`, `PAUSE`, `SEEK`, `STATE_UPDATE` etc.

Se colocarmos duas ou mais réplicas do servidor atrás de um load balancer, um ouvinte conectado na réplica B não recebe o `PLAY` que o DJ enviou para a réplica A. Ou seja: o sistema atual **não escala horizontalmente**.

Também há estado da estação (`PlaybackState`, playlist) guardado só em memória, o que impede qualquer réplica de atender qualquer cliente.

## Decisão

1. Cada réplica do Sync Service publica os eventos de uma estação no canal Redis `station:{id}:events` e assina esse mesmo canal. Ao receber uma mensagem do Redis, a réplica a repassa apenas para os clientes WebSocket **locais** daquela estação (Messaging Fan-out).
2. O `PlaybackState` passa a ser guardado em Redis (`station:{id}:state`), tornando as réplicas *stateless* — uma réplica nova lê o estado quente ao aceitar conexões.
3. Lock global de DJ no Redis com TTL e heartbeat (`station:{id}:dj`), garantindo a regra de 1 DJ por estação globalmente entre réplicas.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Redis Pub/Sub** (escolhida) | Latência sub-milissegundo; simples; o mesmo Redis guarda o estado | Entrega *at-most-once*: mensagem perdida se a réplica estiver desconectada |
| Sticky sessions (todos da estação na mesma réplica) | Nenhuma mudança no código | Estação grande não cabe numa réplica; queda da réplica derruba a estação inteira |
| NATS / Kafka | Garantias de entrega maiores; replay | Mais um componente para operar; replay é desnecessário para eventos de player |
| RabbitMQ fanout exchange | Já teremos RabbitMQ (ADR-0002) | Latência maior e mais overhead por mensagem que Redis |

## Consequências

- **Positivas:** o Sync Service pode ter N réplicas; uma réplica pode cair sem derrubar a estação (os clientes reconectam em outra com `docker compose up -d --scale sync=N`).
- **Negativas / trade-offs:** Pub/Sub é *fire-and-forget*. Mitigação: o cliente recebe o `PlaybackState` completo no handshake e envia `SYNC_REQUEST` ao reconectar, então um evento perdido é corrigido no próximo `STATE_UPDATE`. Redis vira ponto único de falha no fan-out (mitigável com Redis Sentinel/Cluster em produção).

## Implementação (Entrega 3)

A extração do serviço foi concluída com os seguintes componentes:
1. **Novo binário e container:** `backend/cmd/sync` compilado em container independente Alpine (`Dockerfile.sync`), executando o pacote `internal/syncservice`.
2. **Gateway Strangler Fig:** no `docker-compose.yml`, o Traefik roteia `PathPrefix('/ws/')` com prioridade 100 para o serviço `sync`, com load balancing automático entre réplicas.
3. **Métricas Prometheus:** pacote `internal/metrics` exposto em `/metrics` no Sync Service, coletando conexões ativas (`radiosync_sync_active_connections`), total de mensagens (`radiosync_sync_messages_total`) e histograma de latência de fan-out (`radiosync_sync_fanout_duration_seconds`).
4. **Resiliência a reinício do Redis (RES-04):** o `RedisSyncStorage` possui reconexão com backoff exponencial na assinatura do canal `station:{id}:events`.

### Resultados Medidos nos Testes Automatizados

| Teste | Cenário | Meta | Resultado Medido | Situação |
|---|---|---|---|---|
| **INT-01** | PLAY na réplica A propagado para ouvinte na réplica B | < 200 ms | **1,10 ms** | Aprovado |
| **RES-01** | Queda da réplica e reconexão do cliente com estado quente | < 5 s | **1,71 ms** | Aprovado |
| **RES-04** | Reinício do broker e restabelecimento do fan-out | Eventos voltam a fluir | **Confirmado** (re-subscrição com sucesso) | Aprovado |
