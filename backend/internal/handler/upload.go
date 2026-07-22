package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"radio-backend/internal/auth"
	"radio-backend/internal/media"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func UploadMusic(w http.ResponseWriter, r *http.Request) {
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

	r.ParseMultipartForm(50 << 20) // 50MB max

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "arquivo não enviado", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	if !media.IsSupported(ext) {
		http.Error(w, fmt.Sprintf("formato não suportado: %s", ext), http.StatusBadRequest)
		return
	}

	trackID := uuid.New().String()
	tempPath := filepath.Join(os.TempDir(), trackID+ext)
	opusPath := filepath.Join(getMusicDir(), trackID+".opus")

	out, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, "erro ao salvar arquivo", http.StatusInternalServerError)
		return
	}
	io.Copy(out, file)
	out.Close()
	defer os.Remove(tempPath)

	if err := media.ConvertToOpus(tempPath, opusPath); err != nil {
		log.Printf("erro na conversão de %s: %v", header.Filename, err)
		http.Error(w, fmt.Sprintf("erro na conversão: %v", err), http.StatusInternalServerError)
		return
	}

	duration, err := media.GetDuration(opusPath)
	if err != nil {
		duration = 0
	}

	track := model.Track{
		ID:       trackID,
		Title:    header.Filename[:len(header.Filename)-len(ext)],
		Filename: trackID + ".opus",
		URL:      fmt.Sprintf("/musicas/%s.opus", trackID),
		Duration: duration,
	}

	station.Lock()
	station.Repository = append(station.Repository, track)
	station.Unlock()

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(track)
}

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
