package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"radio-backend/internal/breaker"
	"radio-backend/internal/jobs"
	"radio-backend/internal/mediaworker"
	"radio-backend/internal/metrics"
	"radio-backend/internal/queue"
	"radio-backend/internal/storage"
	"radio-backend/internal/syncservice"
	"radio-backend/internal/tracing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	defer tracing.Setup("radiosync-worker")()
	_ = godotenv.Load()

	instanceID := os.Getenv("HOSTNAME")
	if instanceID == "" {
		instanceID = "worker-" + uuid.New().String()[:8]
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// O Station Service é dono do schema e roda as migrations (ADR-0006).
	repo, err := storage.OpenPostgresRepository(ctx, storage.DatabaseURLFromEnv())
	if err != nil {
		log.Fatalf("Media Worker: falha ao conectar no PostgreSQL: %v", err)
	}
	defer repo.Close()

	objects, err := storage.NewMinIOObjectStore(storage.MinIOConfigFromEnv())
	if err != nil {
		log.Fatalf("Media Worker: falha ao inicializar MinIO: %v", err)
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379/0"
	}
	store, err := jobs.NewRedisStore(redisURL)
	if err != nil {
		log.Fatalf("Media Worker: REDIS_URL inválida: %v", err)
	}
	defer store.Close()
	events, err := syncservice.NewRedisSyncStorage(redisURL, instanceID)
	if err != nil {
		log.Fatalf("Media Worker: falha ao conectar no Redis: %v", err)
	}
	defer events.Close()

	retry, err := queue.ParseRetryPolicy(os.Getenv("MEDIA_RETRY_DELAYS"))
	if err != nil {
		log.Fatalf("Media Worker: MEDIA_RETRY_DELAYS: %v", err)
	}
	rabbit := queue.NewRabbit(queue.URLFromEnv(), retry.Delays...)
	defer rabbit.Close()

	worker := mediaworker.New(mediaworker.Config{
		ID:      instanceID,
		Queue:   rabbit,
		Jobs:    store,
		Repo:    repo,
		Objects: objects,
		Events:  events,
		Breaker: mediaworker.NewBreaker(breaker.Config{
			MaxFailures: envInt("YT_BREAKER_FAILURES"),
			Cooldown:    envDuration("YT_BREAKER_COOLDOWN"),
		}),
		Retry:       retry,
		JobTimeout:  envDuration("MEDIA_JOB_TIMEOUT"),
		Concurrency: envInt("WORKER_CONCURRENCY"),
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}
	srv := &http.Server{Addr: ":" + port, Handler: opsHandler(instanceID, rabbit), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Media Worker: %v", err)
		}
	}()

	log.Printf("Media Worker %s consumindo %s (métricas na porta %s)", instanceID, queue.TranscodeQueue, port)
	_ = worker.Run(ctx)
	log.Printf("Media Worker %s encerrado; jobs em andamento devolvidos à fila", instanceID)
}

// opsHandler expõe só /healthz e /metrics, na rede interna. Sem conexão com a
// fila o worker não está saudável.
func opsHandler(instanceID string, rabbit *queue.Rabbit) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status, code := "ok", http.StatusOK
		if !rabbit.Connected() {
			status, code = "fila indisponível", http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": status, "service": "media-worker", "instance": instanceID})
	})
	return mux
}

// envInt e envDuration devolvem zero (o padrão de cada opção) se a variável
// estiver vazia, e param a subida se estiver errada.
func envInt(name string) int {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Fatalf("Media Worker: %s inválido: %q", name, v)
	}
	return n
}

func envDuration(name string) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Fatalf("Media Worker: %s inválido: %q", name, v)
	}
	return d
}
