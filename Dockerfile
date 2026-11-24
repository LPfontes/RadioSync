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

# Sem FFmpeg nem yt-dlp: a conversão roda só no Media Worker (ADR-0002)
FROM alpine:3.21
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates curl
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
# (ADR-0007). O Traefik não roteia para container em "starting", então o
# --start-interval curto faz a versão nova entrar em ~1 s, não em 10 s.
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --start-interval=1s --retries=3 \
  CMD curl -fsS http://localhost:8080/healthz || exit 1
CMD ["./server"]
