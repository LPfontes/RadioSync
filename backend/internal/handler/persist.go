package handler

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"radio-backend/internal/model"
	"radio-backend/internal/ws"
)

type savedStation struct {
	ID          string                 `json:"id"`
	DJ          string                 `json:"dj"`
	State       *model.PlaybackState   `json:"state"`
	Repository  []model.Track          `json:"repository"`
	Playlist    []model.Track          `json:"playlist"`
	Suggestions []model.SongSuggestion `json:"suggestions"`
}

var (
	persistMu sync.Mutex
)

func dataDir() string {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

func persistPath() string {
	return filepath.Join(dataDir(), "stations.json")
}

func SaveStations() {
	stationsMu.RLock()
	all := make([]savedStation, 0, len(stations))
	for _, s := range stations {
		s.RLock()
		all = append(all, savedStation{
			ID:          s.ID,
			DJ:          s.DJ,
			State:       s.State,
			Repository:  s.Repository,
			Playlist:    s.Playlist,
			Suggestions: s.Suggestions,
		})
		s.RUnlock()
	}
	stationsMu.RUnlock()

	persistMu.Lock()
	defer persistMu.Unlock()

	dir := dataDir()
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		log.Printf("erro ao serializar estações: %v", err)
		return
	}

	tmpPath := filepath.Join(dir, "stations.json.tmp")
	finalPath := persistPath()

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		log.Printf("erro ao salvar estações: %v", err)
		return
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		log.Printf("erro ao renomear arquivo de estações: %v", err)
	}
}

func LoadStations() {
	persistMu.Lock()
	defer persistMu.Unlock()

	data, err := os.ReadFile(persistPath())
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("erro ao ler arquivo de estações: %v", err)
		}
		return
	}

	var saved []savedStation
	if err := json.Unmarshal(data, &saved); err != nil {
		log.Printf("erro ao parsear estações: %v", err)
		return
	}

	stationsMu.Lock()
	defer stationsMu.Unlock()

	for _, ss := range saved {
		station := model.NewStation(ss.ID, ss.DJ)
		station.State = ss.State
		station.Repository = ss.Repository
		station.Playlist = ss.Playlist
		if ss.Suggestions != nil {
			station.Suggestions = ss.Suggestions
		} else {
			station.Suggestions = make([]model.SongSuggestion, 0)
		}

		currentStation := station
		currentStation.Hub.OnMessage = func(client *ws.Client, msg []byte) {
			currentStation.HandleMessage(client, msg)
			SaveStations()
		}

		stations[ss.ID] = currentStation
	}

	log.Printf("%d estações restauradas do arquivo", len(saved))
}

func PeriodicSave() {
	for {
		time.Sleep(30 * time.Second)
		SaveStations()
	}
}
