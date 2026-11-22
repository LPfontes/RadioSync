package main

import (
	"context"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/importer"
	"radio-backend/internal/security"
	"radio-backend/internal/stationservice"
	"radio-backend/internal/storage"
	"radio-backend/internal/syncservice"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if warning, err := auth.CheckSecret(); err != nil {
		log.Fatalf("Station Service: erro no segredo JWT: %v", err)
	} else if warning != "" {
		log.Printf("Station Service: AVISO: %s", warning)
	}

	_ = mime.AddExtensionType(".opus", "audio/ogg")

	instanceID := os.Getenv("HOSTNAME")
	if instanceID == "" {
		instanceID = "station-" + uuid.New().String()[:8]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Conecta ao PostgreSQL e executa as migrations versionadas (ADR-0006)
	dbURL := storage.DatabaseURLFromEnv()
	repo, err := storage.NewPostgresRepository(ctx, dbURL)
	if err != nil {
		log.Fatalf("Station Service: falha ao inicializar PostgreSQL: %v", err)
	}
	defer repo.Close()

	// 2. Conecta ao MinIO e garante existência do bucket "musicas" (ADR-0006)
	objStore, err := storage.NewMinIOObjectStore(storage.MinIOConfigFromEnv())
	if err != nil {
		log.Fatalf("Station Service: falha ao inicializar MinIO ObjectStore: %v", err)
	}
	if err := objStore.EnsureBucket(ctx); err != nil {
		log.Printf("Station Service: AVISO ao garantir bucket no MinIO: %v", err)
	}

	// 3. Executa importador idempotente automaticamente na subida caso existam JSONs/.opus locais
	if os.Getenv("AUTO_IMPORT_ON_BOOT") != "false" {
		if rep, err := importer.Run(ctx, importer.Config{}, repo, objStore); err != nil {
			log.Printf("Station Service: AVISO na importação inicial: %v", err)
		} else if rep.StationsImported > 0 || rep.TracksImported > 0 || rep.AudioFilesUploaded > 0 {
			log.Printf("Station Service: importação automática concluída (%+v)", rep)
		}
	}

	// 4. Conecta opcionalmente ao Redis para leitura de estado quente e lock de DJ (ADR-0001)
	var syncStore syncservice.SyncStorage
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379/0"
	}
	if rs, err := syncservice.NewRedisSyncStorage(redisURL, instanceID); err == nil {
		syncStore = rs
		defer rs.Close()
	} else {
		log.Printf("Station Service: AVISO Redis não conectado no boot (%v); operando apenas com Postgres/MinIO", err)
	}

	allowedOrigins := security.AllowedOrigins()
	svc := stationservice.NewService(stationservice.Config{
		InstanceID:        instanceID,
		Repo:              repo,
		ObjectStore:       objStore,
		SyncStorage:       syncStore,
		AudioDeliveryMode: os.Getenv("AUDIO_DELIVERY_MODE"),
		AllowedOrigins:    allowedOrigins,
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           svc.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("Station Service rodando na porta %s (instância %s)", port, instanceID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Station Service falhou: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Printf("Desligando Station Service [%s]...", instanceID)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Println("Station Service encerrado com sucesso.")
}
