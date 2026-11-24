package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/metrics"
	"radio-backend/internal/security"
	"radio-backend/internal/syncservice"
	"radio-backend/internal/tracing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	defer tracing.Setup("radiosync-sync")()
	godotenv.Load()

	// Validação do JWT_SECRET para tokens de DJ (ADR-0005)
	if warning, err := auth.CheckSecret(); err != nil {
		log.Fatalf("Sync Service: erro no segredo JWT: %v", err)
	} else if warning != "" {
		log.Printf("Sync Service: AVISO: %s", warning)
	}

	instanceID := os.Getenv("HOSTNAME")
	if instanceID == "" {
		instanceID = "sync-" + uuid.New().String()[:8]
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379/0"
	}

	log.Printf("Iniciando Sync Service [%s] com Redis: %s", instanceID, redisURL)

	storage, err := syncservice.NewRedisSyncStorage(redisURL, instanceID)
	if err != nil {
		log.Fatalf("Sync Service: falha ao conectar no Redis: %v", err)
	}
	defer storage.Close()

	allowedOrigins := security.AllowedOrigins()
	log.Printf("CORS/WebSocket: mesma origem + ALLOWED_ORIGINS=%v", allowedOrigins)

	syncSvc := syncservice.NewSyncService(storage, instanceID, allowedOrigins)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	// Antes do Recoverer: um panic recuperado como 500 entra na taxa de erro.
	r.Use(tracing.HTTPMiddleware("radiosync-sync"))
	r.Use(metrics.HTTPMiddleware("sync"))
	r.Use(chimw.Recoverer)

	// Métricas Prometheus (tarefa Membro 5 / observabilidade)
	r.Handle("/metrics", metrics.Handler())

	// Healthcheck do serviço e réplica
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "ok",
			"service":  "sync",
			"instance": instanceID,
		})
	})

	// Rota WebSocket roteada pelo API Gateway Traefik (Strangler Fig)
	r.Get("/ws/stations/{stationId}", syncSvc.HandleWebSocket)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Sync Service rodando na porta %s (instância %s)", port, instanceID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Sync Service falhou: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Printf("Desligando Sync Service [%s]...", instanceID)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = srv.Shutdown(shutdownCtx)
	_ = syncSvc.Close()
	log.Println("Sync Service encerrado com sucesso.")
}
