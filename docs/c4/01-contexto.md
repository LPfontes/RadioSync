# C4 — Nível 1: Diagrama de Contexto

Quem usa o RadioSync e com quais sistemas externos ele conversa.

```mermaid
C4Context
    title RadioSync — Contexto do Sistema

    Person(dj, "DJ", "Cria a estação, envia músicas, monta a fila e controla play/pause/seek.")
    Person(ouvinte, "Ouvinte", "Entra na estação pelo código de 6 caracteres e escuta em sincronia.")
    Person(admin, "Administrador", "Gerencia estações e o catálogo global.")

    System(radiosync, "RadioSync", "Rádio web sincronizada em tempo real: distribui o estado do player via WebSocket e serve os arquivos .opus.")

    System_Ext(youtube, "YouTube", "Origem de áudio para importação de faixas via yt-dlp.")
    System_Ext(potprovider, "bgutil PO Token Provider", "Gera tokens exigidos pelo YouTube para o yt-dlp.")

    Rel(dj, radiosync, "Cria estação, faz upload, envia comandos", "HTTPS / WebSocket")
    Rel(ouvinte, radiosync, "Recebe estado do player e baixa faixas", "HTTPS / WebSocket")
    Rel(admin, radiosync, "Administra estações e faixas", "HTTPS")
    Rel(radiosync, youtube, "Baixa áudio", "HTTPS (yt-dlp)")
    Rel(radiosync, potprovider, "Solicita PO token", "HTTP")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## Elementos

| Elemento | Tipo | Responsabilidade |
|---|---|---|
| DJ | Pessoa | Controla a estação. Autenticado por um JWT por estação (`djToken`). |
| Ouvinte | Pessoa | Somente leitura: recebe eventos e toca o áudio localmente. Pode sugerir músicas. |
| Administrador | Pessoa | Acessa `/api/v1/admin/*` (hoje sem autenticação — ver ADR-0005). |
| RadioSync | Sistema | O sistema deste projeto. |
| YouTube | Sistema externo | Fonte de áudio. Instável/bloqueante → protegido por Circuit Breaker (ADR-0002). |
| PO Token Provider | Sistema externo | Container auxiliar exigido pelo yt-dlp. |
