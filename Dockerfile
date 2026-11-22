FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o server ./cmd/server/

FROM denoland/deno:alpine
RUN apk add --no-cache ffmpeg ca-certificates python3 py3-pip curl && \
    curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp && \
    pip install --break-system-packages -U bgutil-ytdlp-pot-provider yt-dlp-ejs
WORKDIR /app
COPY --from=backend /app/server .
COPY --from=frontend /app/dist ./frontend/dist
RUN mkdir -p /app/musicas /app/data

ENV PORT=8080
ENV MUSIC_DIR=/app/musicas
ENV FRONTEND_DIR=/app/frontend/dist
ENV DATA_DIR=/app/data
EXPOSE 8080
# O deploy só troca o tráfego para a versão nova depois de /healthz responder
# (ADR-0007). curl já está na imagem.
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://localhost:8080/healthz || exit 1
CMD ["./server"]
