# ADR-0004: Sincronização de áudio no cliente por timestamp (sem streaming)

- **Status:** Aceito (já implementado)
- **Data:** 2026-11-03 (registro retroativo de decisão do início do projeto)
- **Responsável:** Membro 1 (Sync Service)
- **Áreas:** Desempenho, Escalabilidade

## Contexto

Uma rádio sincronizada precisa que todos os ouvintes escutem o mesmo trecho da música ao mesmo tempo. A forma "clássica" é o servidor transmitir o áudio continuamente (Icecast, HLS ao vivo), o que custa banda e CPU proporcional ao número de ouvintes.

## Decisão

O servidor **não transmite áudio**. Ele só distribui, via WebSocket, um estado pequeno (`PlaybackState`: `isPlaying`, `startedAt`, `seekOffset`, `currentSong`). Cada cliente baixa o arquivo `.opus` como arquivo estático e calcula sozinho:

```
posição = (agora_cliente − startedAt) / 1000 + seekOffset
```

Se a diferença para o `currentTime` do `<audio>` passar de ~1 s, o player corrige. O cálculo equivalente no servidor está em `Station.calcPosition()` (`backend/internal/model/station.go`) e tem teste unitário.

Complementos já implementados: arquivos em **Opus 96 kbps VBR** (menos banda) e **janela deslizante** — o cliente só recebe as 3 próximas faixas da playlist e faz pre-buffer delas.

## Alternativas consideradas

| Alternativa | Prós | Contras |
|---|---|---|
| **Timestamp + arquivo estático** (escolhida) | Custo do servidor quase não cresce com ouvintes; arquivos cacheáveis (CDN) | Depende do relógio do cliente; precisão de ~1 s |
| Streaming ao vivo (Icecast/HLS) | Sincronização "natural" | Banda e CPU por ouvinte; latência de vários segundos no HLS |
| WebRTC | Latência muito baixa | Complexo (SFU, TURN); caro para muitos ouvintes |

## Consequências

- **Positivas:** o caminho crítico (WebSocket) carrega só mensagens de poucos bytes, o que torna viável o fan-out entre réplicas (ADR-0001). Os `.opus` podem ir para armazenamento de objetos (ADR-0006).
- **Negativas / trade-offs:** relógios de clientes desajustados causam desvio. Melhoria planejada: estimar o offset do relógio com o `serverTime` da mensagem `SYNC` (estilo NTP: `offset = serverTime − (t0 + t1)/2`).
- **Como vamos validar:** teste com N clientes simulados medindo o desvio máximo entre a posição calculada por cada um (meta: < 500 ms em p95).
