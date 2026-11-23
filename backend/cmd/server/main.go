package main

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

	handler.SaveOnShutdown()

	fileServer := http.FileServer(http.Dir(musicDir))
	r.Handle("/musicas/*", http.StripPrefix("/musicas/", fileServer))
	// Saúde e versão em execução, para o HEALTHCHECK e o deploy (ADR-0007)
	r.Get("/healthz", handler.Healthz)

	r.Route("/api/v1/stations", func(r chi.Router) {
		r.Post("/", handler.CreateStation)
		r.Get("/{stationId}", handler.GetStation)
		// upload, youtube, jobs e cookies: Media API (cmd/media, ADR-0002)
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

	// Timeouts contra Slowloris (gosec G114): quem abre conexão e manda o
	// cabeçalho devagar segura uma goroutine e um socket para sempre. Só o
	// cabeçalho e a ociosidade têm limite; ReadTimeout/WriteTimeout cortariam
	// uploads grandes e as conexões WebSocket, que são longas por natureza.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("Servidor rodando na porta %s", port)
	log.Fatal(srv.ListenAndServe())
}
