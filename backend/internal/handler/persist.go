package handler

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"radio-backend/internal/auth"
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
	persistMu       sync.Mutex
	tracksCatalog   = make(map[string]model.Track)
	tracksCatalogMu sync.RWMutex
)

func init() {
	model.TrackRegisterer = RegisterOrUpdateTrackMetadata
	model.SaveStationsFunc = SaveStations
}

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

func tracksPersistPath() string {
	return filepath.Join(dataDir(), "tracks.json")
}

func RegisterOrUpdateTrackMetadata(t model.Track) {
	if t.ID == "" {
		return
	}
	tracksCatalogMu.Lock()
	tracksCatalog[t.ID] = t
	tracksCatalogMu.Unlock()
	go SaveTracksCatalog()
}

func GetTrackMetadata(trackID string) (model.Track, bool) {
	tracksCatalogMu.RLock()
	defer tracksCatalogMu.RUnlock()
	t, ok := tracksCatalog[trackID]
	return t, ok
}

func GetAllCatalogTracks() []model.Track {
	tracksCatalogMu.RLock()
	defer tracksCatalogMu.RUnlock()
	list := make([]model.Track, 0, len(tracksCatalog))
	for _, t := range tracksCatalog {
		list = append(list, t)
	}
	return list
}

func SaveTracksCatalog() {
	tracksCatalogMu.RLock()
	all := make([]model.Track, 0, len(tracksCatalog))
	for _, t := range tracksCatalog {
		all = append(all, t)
	}
	tracksCatalogMu.RUnlock()

	persistMu.Lock()
	defer persistMu.Unlock()

	dir := dataDir()
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		log.Printf("erro ao serializar catálogo de músicas: %v", err)
		return
	}

	tmpPath := filepath.Join(dir, "tracks.json.tmp")
	finalPath := tracksPersistPath()

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		log.Printf("erro ao salvar catálogo de músicas: %v", err)
		return
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		log.Printf("erro ao renomear arquivo de catálogo de músicas: %v", err)
	}
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

func LoadTracksCatalog() {
	persistMu.Lock()
	defer persistMu.Unlock()

	data, err := os.ReadFile(tracksPersistPath())
	if err == nil {
		var saved []model.Track
		if err := json.Unmarshal(data, &saved); err == nil {
			tracksCatalogMu.Lock()
			for _, t := range saved {
				if t.ID != "" {
					tracksCatalog[t.ID] = t
				}
			}
			tracksCatalogMu.Unlock()
			log.Printf("%d metadados de músicas restaurados de tracks.json", len(saved))
			return
		}
	}

	// Se tracks.json não existir, migra dos repositórios de estações salvos
	stationsMu.RLock()
	tracksCatalogMu.Lock()
	migrated := 0
	for _, s := range stations {
		s.RLock()
		for _, t := range s.Repository {
			if t.ID != "" {
				if _, exists := tracksCatalog[t.ID]; !exists {
					tracksCatalog[t.ID] = t
					migrated++
				}
			}
		}
		s.RUnlock()
	}
	tracksCatalogMu.Unlock()
	stationsMu.RUnlock()

	if migrated > 0 {
		log.Printf("%d metadados de músicas migrados das estações ativas", migrated)
		go SaveTracksCatalog()
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

	tracksCatalogMu.Lock()
	for _, ss := range saved {
		djToken := ss.DJ
		if !auth.ValidateDJToken(djToken, ss.ID) {
			if newToken, err := auth.GenerateDJToken(ss.ID); err == nil {
				djToken = newToken
			}
		}

		station := model.NewStation(ss.ID, djToken)
		station.State = ss.State
		station.Repository = ss.Repository
		station.Playlist = ss.Playlist
		if ss.Suggestions != nil {
			station.Suggestions = ss.Suggestions
		} else {
			station.Suggestions = make([]model.SongSuggestion, 0)
		}

		for _, t := range ss.Repository {
			if t.ID != "" {
				if _, exists := tracksCatalog[t.ID]; !exists {
					tracksCatalog[t.ID] = t
				}
			}
		}

		currentStation := station
		currentStation.Hub.OnMessage = func(client *ws.Client, msg []byte) {
			currentStation.HandleMessage(client, msg)
			SaveStations()
		}

		stations[ss.ID] = currentStation
	}
	tracksCatalogMu.Unlock()

	log.Printf("%d estações restauradas do arquivo", len(saved))
}

func PeriodicSave() {
	for {
		time.Sleep(30 * time.Second)
		SaveStations()
		SaveTracksCatalog()
	}
}

