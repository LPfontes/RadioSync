package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"radio-backend/internal/importer"
	"radio-backend/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	dataDir := flag.String("data-dir", os.Getenv("DATA_DIR"), "Diretório dos arquivos JSON (stations.json, tracks.json, yt_cache.json)")
	musicDir := flag.String("music-dir", os.Getenv("MUSIC_DIR"), "Diretório dos arquivos .opus e track_names.json")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	repo, err := storage.NewPostgresRepository(ctx, storage.DatabaseURLFromEnv())
	if err != nil {
		log.Fatalf("Erro ao conectar ao PostgreSQL: %v", err)
	}
	defer repo.Close()

	store, err := storage.NewMinIOObjectStore(storage.MinIOConfigFromEnv())
	if err != nil {
		log.Fatalf("Erro ao configurar cliente MinIO: %v", err)
	}

	report, err := importer.Run(ctx, importer.Config{
		DataDir:  *dataDir,
		MusicDir: *musicDir,
	}, repo, store)
	if err != nil {
		log.Fatalf("Falha na importação: %v", err)
	}

	out, _ := json.MarshalIndent(report, "", "  ")
	log.Printf("Relatório de importação:\n%s", string(out))
}
