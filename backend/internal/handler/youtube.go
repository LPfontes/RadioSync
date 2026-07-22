package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"radio-backend/internal/auth"
	"radio-backend/internal/media"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type YouTubeDownloadRequest struct {
	URL string `json:"url"`
}

func DownloadYouTubeHandler(w http.ResponseWriter, r *http.Request) {
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

	var req YouTubeDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "URL do YouTube inválida ou ausente", http.StatusBadRequest)
		return
	}

	trackID := uuid.New().String()
	opusPath := fmt.Sprintf("%s/%s.opus", getMusicDir(), trackID)

	title, duration, err := media.DownloadYouTubeAudio(req.URL, opusPath)
	if err != nil {
		log.Printf("erro ao baixar do YouTube para estação %s: %v", stationID, err)
		http.Error(w, fmt.Sprintf("erro no download do YouTube: %v", err), http.StatusInternalServerError)
		return
	}

	track := model.Track{
		ID:       trackID,
		Title:    title,
		Filename: trackID + ".opus",
		URL:      fmt.Sprintf("/musicas/%s.opus", trackID),
		Duration: duration,
	}

	station.Lock()
	station.Repository = append(station.Repository, track)
	station.Unlock()

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(track)
}
