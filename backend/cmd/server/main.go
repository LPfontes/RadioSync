package main

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"radio-backend/internal/handler"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	godotenv.Load()

	err := mime.AddExtensionType(".opus", "audio/ogg")
	if err != nil {
		log.Fatal("Erro MIME Opus:", err)
	}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
	r.Use(corsHandler.Handler)

	musicDir := os.Getenv("MUSIC_DIR")
	if musicDir == "" {
		musicDir = "../musicas"
	}
	musicDir, _ = filepath.Abs(musicDir)
	os.MkdirAll(musicDir, 0755)
	log.Printf("Diretório de músicas: %s", musicDir)

	handler.LoadStations()
	go handler.PeriodicSave()

	fileServer := http.FileServer(http.Dir(musicDir))
	r.Handle("/musicas/*", http.StripPrefix("/musicas/", fileServer))

	r.Route("/api/v1/stations", func(r chi.Router) {
		r.Post("/", handler.CreateStation)
		r.Get("/{stationId}", handler.GetStation)
		r.Post("/{stationId}/upload", handler.UploadMusic)
		r.Post("/{stationId}/youtube", handler.DownloadYouTubeHandler)
		r.Get("/{stationId}/repository", handler.GetRepository)
		r.Get("/{stationId}/musicas", handler.ListMusicFiles)
		r.Get("/{stationId}/library", handler.GetGlobalLibrary)
	})

	r.Get("/api/v1/library", handler.GetGlobalLibrary)
	r.Get("/api/v1/debug", handler.DebugHandler)

	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Get("/stations", handler.GetAdminStations)
		r.Delete("/stations/{stationId}", handler.DeleteStationAdmin)
		r.Post("/purge-orphans", handler.PurgeOrphanTracksAdmin)
		r.Delete("/stations/{stationId}/tracks/{trackId}", handler.RemoveTrackFromStationAdmin)
	})

	r.Get("/ws/stations/{stationId}", handler.HandleWebSocket)

	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = "../frontend/dist"
	}
	frontendDir, _ = filepath.Abs(frontendDir)
	log.Printf("Diretório frontend: %s", frontendDir)

	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := filepath.Join(frontendDir, r.URL.Path)
		if _, err := os.Stat(filePath); err == nil {
			http.ServeFile(w, r, filePath)
			return
		}
		http.ServeFile(w, r, filepath.Join(frontendDir, "index.html"))
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Servidor rodando na porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
