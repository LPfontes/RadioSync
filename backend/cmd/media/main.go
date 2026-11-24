package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/jobs"
	"radio-backend/internal/mediaservice"
	"radio-backend/internal/metrics"
	"radio-backend/internal/queue"
	"radio-backend/internal/security"
	"radio-backend/internal/storage"
	"radio-backend/internal/tracing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	defer tracing.Setup("radiosync-media")()
	_ = godotenv.Load()

	if warning, err := auth.CheckSecret(); err != nil {
		log.Fatalf("Media API: erro no segredo JWT: %v", err)
	} else if warning != "" {
		log.Printf("Media API: AVISO: %s", warning)
	}

	instanceID := os.Getenv("HOSTNAME")
	if instanceID == "" {
		instanceID = "media-" + uuid.New().String()[:8]
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// O Station Service é dono do schema e roda as migrations (ADR-0006).
	repo, err := storage.OpenPostgresRepository(ctx, storage.DatabaseURLFromEnv())
	if err != nil {
		log.Fatalf("Media API: falha ao conectar no PostgreSQL: %v", err)
	}
	defer repo.Close()

	objects, err := storage.NewMinIOObjectStore(storage.MinIOConfigFromEnv())
	if err != nil {
		log.Fatalf("Media API: falha ao inicializar MinIO: %v", err)
	}
	if err := objects.EnsureBucket(ctx); err != nil {
		log.Printf("Media API: AVISO ao garantir bucket no MinIO: %v", err)
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379/0"
	}
	store, err := jobs.NewRedisStore(redisURL)
	if err != nil {
		log.Fatalf("Media API: REDIS_URL inválida: %v", err)
	}
	defer store.Close()

	retry, err := queue.ParseRetryPolicy(os.Getenv("MEDIA_RETRY_DELAYS"))
	if err != nil {
		log.Fatalf("Media API: MEDIA_RETRY_DELAYS: %v", err)
	}
	rabbit := queue.NewRabbit(queue.URLFromEnv(), retry.Delays...)
	defer rabbit.Close()
	if err := rabbit.Connect(); err != nil {
		log.Printf("Media API: AVISO: %v (tentando de novo a cada pedido)", err)
	}
	go reportDepths(ctx, rabbit)

	maxUploadMB, _ := strconv.ParseInt(os.Getenv("MEDIA_MAX_UPLOAD_MB"), 10, 64)
	svc := mediaservice.NewService(mediaservice.Config{
		InstanceID:     instanceID,
		Repo:           repo,
		Objects:        objects,
		Queue:          rabbit,
		Jobs:           store,
		AllowedOrigins: security.AllowedOrigins(),
		MaxUpload:      maxUploadMB << 20,
		QueueConnected: rabbit.Connected,
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           svc.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		log.Printf("Media API rodando na porta %s (instância %s)", port, instanceID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Media API falhou: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("Desligando Media API [%s]...", instanceID)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// reportDepths publica a profundidade das filas em /metrics.
func reportDepths(ctx context.Context, rabbit *queue.Rabbit) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if depths, err := rabbit.Depths(); err == nil {
			for name, n := range depths {
				metrics.MediaQueueDepth.WithLabelValues(name).Set(float64(n))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
