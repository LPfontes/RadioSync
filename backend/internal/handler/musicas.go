package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"radio-backend/internal/auth"
	"radio-backend/internal/media"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
)

func init() {
	model.TrackResolver = FindGlobalTrack
}

func getMusicDir() string {
	dir := os.Getenv("MUSIC_DIR")
	if dir == "" {
		dir = "../musicas"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

func GetAllLibraryTracks() []model.Track {
	trackMap := make(map[string]model.Track)
	dir := getMusicDir()

	stationsMu.RLock()
	for _, s := range stations {
		s.RLock()
		for _, t := range s.Repository {
			filePath := filepath.Join(dir, t.Filename)
			if _, err := os.Stat(filePath); err == nil {
				if _, exists := trackMap[t.ID]; !exists {
					trackMap[t.ID] = t
				}
			}
		}
		s.RUnlock()
	}
	stationsMu.RUnlock()

	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".opus" {
				filename := e.Name()
				trackID := strings.TrimSuffix(filename, ".opus")

				found := false
				for _, t := range trackMap {
					if t.Filename == filename || t.ID == trackID {
						found = true
						break
					}
				}
				if !found {
					duration, _ := media.GetDuration(filepath.Join(dir, filename))
					t := model.Track{
						ID:       trackID,
						Title:    "Música " + trackID,
						Filename: filename,
						URL:      fmt.Sprintf("/musicas/%s", filename),
						Duration: duration,
					}
					trackMap[t.ID] = t
				}
			}
		}
	}

	result := make([]model.Track, 0, len(trackMap))
	for _, t := range trackMap {
		result = append(result, t)
	}
	return result
}

func FindGlobalTrack(trackID string) (model.Track, bool) {
	tracks := GetAllLibraryTracks()
	for _, t := range tracks {
		if t.ID == trackID || t.Filename == trackID || t.Filename == trackID+".opus" {
			return t, true
		}
	}
	return model.Track{}, false
}

func GetGlobalLibrary(w http.ResponseWriter, r *http.Request) {
	tracks := GetAllLibraryTracks()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}

func ListMusicFiles(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")

	stationsMu.RLock()
	_, ok := stations[stationID]
	stationsMu.RUnlock()
	if !ok {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}

	dir := getMusicDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "erro ao ler diretório", http.StatusInternalServerError)
		return
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".opus" {
			files = append(files, e.Name())
		}
	}
	if files == nil {
		files = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

type UpdateTrackRequest struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Theme    string `json:"theme"`
}

func UpdateTrackMetadata(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	trackID := chi.URLParam(r, "trackId")

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

	var req UpdateTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "payload inválido", http.StatusBadRequest)
		return
	}

	station.Lock()
	defer station.Unlock()

	found := false
	var updatedTrack model.Track

	for i, t := range station.Repository {
		if t.ID == trackID {
			if strings.TrimSpace(req.Title) != "" {
				station.Repository[i].Title = strings.TrimSpace(req.Title)
			}
			station.Repository[i].Category = strings.TrimSpace(req.Category)
			station.Repository[i].Theme = strings.TrimSpace(req.Theme)
			updatedTrack = station.Repository[i]
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "música não encontrada no repositório", http.StatusNotFound)
		return
	}

	for i, t := range station.Playlist {
		if t.ID == trackID {
			station.Playlist[i].Title = updatedTrack.Title
			station.Playlist[i].Category = updatedTrack.Category
			station.Playlist[i].Theme = updatedTrack.Theme
		}
	}

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedTrack)
}

