package main

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"radio-backend/internal/auth"
	"radio-backend/internal/handler"
	"radio-backend/internal/security"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	// Sem segredo forte não há como confiar nos tokens de DJ (ADR-0005).
	if warning, err := auth.CheckSecret(); err != nil {
		log.Fatal(err)
	} else if warning != "" {
		log.Printf("AVISO: %s", warning)
	}

	err := mime.AddExtensionType(".opus", "audio/ogg")
	if err != nil {
		log.Fatal("Erro MIME Opus:", err)
	}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// CORS e Origin do WebSocket restritos (ADR-0005): mesma origem por
	// padrão; outras origens só via ALLOWED_ORIGINS.
	allowedOrigins := security.AllowedOrigins()
	log.Printf("CORS/WebSocket: mesma origem + ALLOWED_ORIGINS=%v", allowedOrigins)
	r.Use(security.CORS(allowedOrigins))
	handler.SetWebSocketOriginCheck(security.CheckWebSocketOrigin(allowedOrigins))

	musicDir := os.Getenv("MUSIC_DIR")
	if musicDir == "" {
		musicDir = "../musicas"
	}
	musicDir, _ = filepath.Abs(musicDir)
	os.MkdirAll(musicDir, 0755)
	log.Printf("Diretório de músicas: %s", musicDir)

	handler.LoadTracksCatalog()
	handler.LoadStations()
	go handler.PeriodicSave()

	fileServer := http.FileServer(http.Dir(musicDir))
	r.Handle("/musicas/*", http.StripPrefix("/musicas/", fileServer))

	r.Route("/api/v1/stations", func(r chi.Router) {
		r.Post("/", handler.CreateStation)
		r.Get("/{stationId}", handler.GetStation)
		r.Post("/{stationId}/upload", handler.UploadMusic)
		r.Post("/{stationId}/youtube", handler.DownloadYouTubeHandler)
		r.Post("/{stationId}/cookies", handler.UploadCookiesHandler)
		r.Get("/{stationId}/cookies/status", handler.GetCookiesStatusHandler)
		r.Get("/{stationId}/repository", handler.GetRepository)
		r.Get("/{stationId}/musicas", handler.ListMusicFiles)
		r.Get("/{stationId}/library", handler.GetGlobalLibrary)
		r.Post("/{stationId}/claim-dj", handler.ClaimDJRole)
		r.Put("/{stationId}/tracks/{trackId}", handler.UpdateTrackMetadata)
	})

	r.Get("/api/v1/library", handler.GetGlobalLibrary)
	// Diagnóstico expõe caminhos e IDs de todas as estações: só para admin.
	r.With(handler.RequireAdmin).Get("/api/v1/debug", handler.DebugHandler)

	if !auth.AdminEnabled() {
		log.Printf("ADMIN_PASSWORD não definido: rotas /api/v1/admin desabilitadas (503)")
	}

	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Post("/login", handler.AdminLogin)

		r.Group(func(r chi.Router) {
			r.Use(handler.RequireAdmin)
			r.Get("/verify", handler.AdminVerify)
			r.Get("/stations", handler.GetAdminStations)
			r.Delete("/stations/{stationId}", handler.DeleteStationAdmin)
			r.Post("/stations/{stationId}/dj-token", handler.IssueDJTokenAdmin)
			r.Post("/purge-orphans", handler.PurgeOrphanTracksAdmin)
			r.Delete("/stations/{stationId}/tracks/{trackId}", handler.RemoveTrackFromStationAdmin)
			r.Put("/tracks/{trackId}", handler.RenameTrackAdmin)
			r.Post("/cookies", handler.SaveCookiesAdmin)
		})
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
