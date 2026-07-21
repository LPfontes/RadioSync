FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM golang:1.26-alpine AS backend
RUN apk add --no-cache ffmpeg
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o server ./cmd/server/

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=backend /app/server .
COPY --from=frontend /app/dist ./frontend/dist
RUN mkdir -p /app/musicas

ENV PORT=8080
ENV MUSIC_DIR=/app/musicas
ENV FRONTEND_DIR=/app/frontend/dist
EXPOSE 8080
CMD ["./server"]
