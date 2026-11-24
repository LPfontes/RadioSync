package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
)

func GetRepository(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")

	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader || !auth.ValidateDJToken(token, stationID) {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}

	stationsMu.RLock()
	station, ok := stations[stationID]
	stationsMu.RUnlock()
	if !ok {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}

	station.RLock()
	rawRepo := station.Repository
	station.RUnlock()

	dir := getMusicDir()
	validRepo := make([]model.Track, 0, len(rawRepo))
	for _, t := range rawRepo {
		if _, err := os.Stat(filepath.Join(dir, t.Filename)); err == nil {
			validRepo = append(validRepo, t)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(validRepo)
}
