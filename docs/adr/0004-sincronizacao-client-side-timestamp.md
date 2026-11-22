# ADR-0004: Sincronização de áudio no cliente por timestamp (sem streaming)

- **Status:** Implementado (Entrega 3)
- **Data:** 2026-11-03 (atualizado em 2026-11-22)
- **Responsável:** Membro 1 — Nathan (Sync Service)
- **Áreas:** Desempenho, Escalabilidade

## Contexto

Uma rádio sincronizada precisa que todos os ouvintes escutem o mesmo trecho da música ao mesmo tempo. A forma "clássica" é o servidor transmitir o áudio continuamente (Icecast, HLS ao vivo), o que custa banda e CPU proporcional ao número de ouvintes.

## Decisão

O servidor **não transmite áudio**. Ele só distribui, via WebSocket, um estado pequeno (`PlaybackState`: `isPlaying`, `startedAt`, `seekOffset`, `currentSong`). Cada cliente baixa o arquivo `.opus` como arquivo estático e calcula sozinho:

```
posição = (agora_estimado_servidor − startedAt) / 1000 + seekOffset
```

Se a diferença para o `currentTime` do `<audio>` passar de ~1 s, o player corrige. O cálculo equivalente no servidor está em `PlaybackState.CalcPosition()` (`backend/internal/model/station.go`) e tem teste unitário.

Complementos implementados: arquivos em **Opus 96 kbps VBR** (menos banda), **janela deslizante** e **correção de offset de relógio estilo NTP** via mensagem `SYNC`.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Timestamp + arquivo estático** (escolhida) | Custo do servidor quase não cresce com ouvintes; arquivos cacheáveis (CDN) | Depende do relógio do cliente; precisão de ~1 s sem correção |
| Streaming ao vivo (Icecast/HLS) | Sincronização "natural" | Banda e CPU por ouvinte; latência de vários segundos no HLS |
| WebRTC | Latência muito baixa | Complexo (SFU, TURN); caro para muitos ouvintes |

## Consequências

- **Positivas:** o caminho crítico (WebSocket) carrega só mensagens de poucos bytes, o que torna viável o fan-out entre réplicas (ADR-0001). Os `.opus` podem ir para armazenamento de objetos (ADR-0006).
- **Negativas / trade-offs:** relógios de clientes desajustados causavam desvio. Mitigado na Entrega 3 através do algoritmo NTP client-side.

## Implementação (Entrega 3)

Na Entrega 3, foi implementada a compensação de clock drift (offset de relógio) entre cliente e servidor:
1. **Envio do timestamp pelo servidor:** ao responder `SYNC_REQUEST`, o servidor anexa `serverTime: time.Now().UnixMilli()`.
2. **Cálculo de offset estilo NTP no frontend:**
   - Ao disparar o `SYNC_REQUEST`, o cliente anota `t0 = Date.now()`.
   - Ao receber o `SYNC`, o cliente anota `t1 = Date.now()`.
   - Estimativa do offset de relógio:
     ```javascript
     const clientMid = (t0 + t1) / 2
     store.clockOffset = msg.serverTime - clientMid
     ```
3. **Aplicação no `usePlayer.js`:**
   ```javascript
   const estimatedServerNow = Date.now() + (store.clockOffset || 0)
   const elapsed = (estimatedServerNow - store.state.startedAt) / 1000
   const expectedPosition = elapsed + store.state.seekOffset
   ```

### Resultados Medidos nos Testes Unitários (`internal/ws/offset_test.go`)

| Cenário de Teste | Relógio do Cliente | RTT | Offset Estimado | Posição Calculada vs. Real | Situação |
|---|---|---|---|---|---|
| Relógio sincronizado | 0 ms drift | 40 ms | 0 ms | 10,000 s / 10,000 s | Aprovado |
| Cliente atrasado em 3s | -3000 ms | 40 ms | +3000 ms | 10,000 s / 10,000 s (sem erro) | Aprovado |
| Cliente adiantado em 2s | +2000 ms | 40 ms | -2000 ms | 10,000 s / 10,000 s (sem erro) | Aprovado |
| Assimetria de rede leve | 0 ms drift | 60 ms | +5 ms | 10,005 s (~5 ms desvio, imperceptível) | Aprovado |
