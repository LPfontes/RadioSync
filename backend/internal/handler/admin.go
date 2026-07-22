package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
)

type AdminStationSummary struct {
	ID                string               `json:"id"`
	DJToken           string               `json:"djToken"`
	State             *model.PlaybackState `json:"state"`
	Repository        []model.Track        `json:"repository"`
	Playlist          []model.Track        `json:"playlist"`
	TrackCount        int                  `json:"trackCount"`
	PlaylistCount     int                  `json:"playlistCount"`
	MissingTrackCount int                  `json:"missingTrackCount"`
	ActiveClients     int                  `json:"activeClients"`
}

func GetAdminStations(w http.ResponseWriter, r *http.Request) {
	stationsMu.RLock()
	defer stationsMu.RUnlock()

	dir := getMusicDir()
	list := make([]AdminStationSummary, 0, len(stations))
	totalTracks := 0
	totalMissing := 0

	for id, s := range stations {
		s.RLock()
		repo := append([]model.Track{}, s.Repository...)
		playlist := append([]model.Track{}, s.Playlist...)
		state := s.State
		s.RUnlock()

		missingCount := 0
		for _, t := range repo {
			if _, err := os.Stat(filepath.Join(dir, t.Filename)); err != nil {
				missingCount++
			}
		}

		activeClients := 0
		if s.Hub != nil {
			activeClients = s.Hub.ClientCount()
		}

		summary := AdminStationSummary{
			ID:                id,
			DJToken:           s.DJ,
			State:             state,
			Repository:        repo,
			Playlist:          playlist,
			TrackCount:        len(repo),
			PlaylistCount:     len(playlist),
			MissingTrackCount: missingCount,
			ActiveClients:     activeClients,
		}
		list = append(list, summary)

		totalTracks += len(repo)
		totalMissing += missingCount
	}

	persistFile := persistPath()
	fileInfo, _ := os.Stat(persistFile)
	fileSize := int64(0)
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stations":          list,
		"totalStations":     len(list),
		"totalTracks":       totalTracks,
		"totalMissingFiles": totalMissing,
		"persistFilePath":   persistFile,
		"persistFileSize":   fileSize,
	})
}

func DeleteStationAdmin(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")

	stationsMu.Lock()
	station, ok := stations[stationID]
	if !ok {
		stationsMu.Unlock()
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}
	delete(stations, stationID)
	stationsMu.Unlock()

	if station.Hub != nil {
		// Close hub connections if any
	}

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "estação excluída com sucesso",
		"stationId": stationID,
	})
}

func PurgeOrphanTracksAdmin(w http.ResponseWriter, r *http.Request) {
	dir := getMusicDir()
	purgedCount := 0

	stationsMu.Lock()
	for _, s := range stations {
		s.Lock()

		// Clean repository
		validRepo := make([]model.Track, 0, len(s.Repository))
		for _, t := range s.Repository {
			if _, err := os.Stat(filepath.Join(dir, t.Filename)); err == nil {
				validRepo = append(validRepo, t)
			} else {
				purgedCount++
			}
		}
		s.Repository = validRepo

		// Clean playlist
		validPlaylist := make([]model.Track, 0, len(s.Playlist))
		for _, t := range s.Playlist {
			if _, err := os.Stat(filepath.Join(dir, t.Filename)); err == nil {
				validPlaylist = append(validPlaylist, t)
			}
		}
		s.Playlist = validPlaylist

		s.Unlock()
	}
	stationsMu.Unlock()

	SaveStations()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"purgedTracks": purgedCount,
		"message":      "metadados de músicas inexistentes removidos de stations.json",
	})
}

func RemoveTrackFromStationAdmin(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	trackID := chi.URLParam(r, "trackId")

	stationsMu.RLock()
	station, ok := stations[stationID]
	stationsMu.RUnlock()

	if !ok {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}

	station.Lock()
	validRepo := make([]model.Track, 0, len(station.Repository))
	for _, t := range station.Repository {
		if t.ID != trackID {
			validRepo = append(validRepo, t)
		}
	}
	station.Repository = validRepo

	validPlaylist := make([]model.Track, 0, len(station.Playlist))
	for _, t := range station.Playlist {
		if t.ID != trackID {
			validPlaylist = append(validPlaylist, t)
		}
	}
	station.Playlist = validPlaylist
	station.Unlock()

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "música removida da estação",
	})
}
