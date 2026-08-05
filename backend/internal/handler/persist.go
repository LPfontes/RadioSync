package handler

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
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

func trackNamesPersistPath() string {
	return filepath.Join(getMusicDir(), "track_names.json")
}

func RegisterOrUpdateTrackMetadata(t model.Track) {
	if t.ID == "" && t.Filename == "" {
		return
	}
	if t.ID == "" {
		t.ID = strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
	}
	tracksCatalogMu.Lock()
	tracksCatalog[t.ID] = t
	if t.Filename != "" {
		base := strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
		tracksCatalog[base] = t
		tracksCatalog[t.Filename] = t
	}
	tracksCatalogMu.Unlock()
	go SaveTracksCatalog()
}

func GetTrackMetadata(trackID string) (model.Track, bool) {
	tracksCatalogMu.RLock()
	defer tracksCatalogMu.RUnlock()
	if t, ok := tracksCatalog[trackID]; ok {
		return t, true
	}
	base := strings.TrimSuffix(trackID, ".opus")
	if t, ok := tracksCatalog[base]; ok {
		return t, true
	}
	if t, ok := tracksCatalog[trackID+".opus"]; ok {
		return t, true
	}
	return model.Track{}, false
}

func GetAllCatalogTracks() []model.Track {
	tracksCatalogMu.RLock()
	defer tracksCatalogMu.RUnlock()
	seen := make(map[string]bool)
	list := make([]model.Track, 0, len(tracksCatalog))
	for _, t := range tracksCatalog {
		if t.ID != "" && !seen[t.ID] {
			seen[t.ID] = true
			list = append(list, t)
		}
	}
	return list
}

func SaveTracksCatalog() {
	tracksCatalogMu.RLock()
	catalogMap := make(map[string]model.Track)
	namesMap := make(map[string]string)
	for _, t := range tracksCatalog {
		if t.ID != "" && t.Title != "" {
			catalogMap[t.ID] = t
			namesMap[t.ID] = t.Title
			if t.Filename != "" {
				base := strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
				namesMap[base] = t.Title
			}
		}
	}
	tracksCatalogMu.RUnlock()

	persistMu.Lock()
	defer persistMu.Unlock()

	dir := dataDir()
	os.MkdirAll(dir, 0755)

	// 1. Salva tracks.json com metadados completos em formato mapa (chave: codigo/ID -> valor: Track)
	data, err := json.MarshalIndent(catalogMap, "", "  ")
	if err != nil {
		log.Printf("erro ao serializar catálogo de músicas: %v", err)
	} else {
		tmpPath := filepath.Join(dir, "tracks.json.tmp")
		finalPath := tracksPersistPath()
		if err := os.WriteFile(tmpPath, data, 0644); err == nil {
			_ = os.Rename(tmpPath, finalPath)
		}
	}

	// 2. Salva track_names.json na pasta de músicas (MUSIC_DIR) em formato JSON chave-valor (chave: codigo da musica -> valor: nome da musica)
	musicDir := getMusicDir()
	os.MkdirAll(musicDir, 0755)
	namesData, err := json.MarshalIndent(namesMap, "", "  ")
	if err != nil {
		log.Printf("erro ao serializar mapa de nomes de músicas: %v", err)
	} else {
		tmpPath := filepath.Join(musicDir, "track_names.json.tmp")
		finalPath := trackNamesPersistPath()
		if err := os.WriteFile(tmpPath, namesData, 0644); err == nil {
			_ = os.Rename(tmpPath, finalPath)
		}
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

	// 1. Carrega de track_names.json em MUSIC_DIR (ou fallback em DATA_DIR se não existir)
	namesPath := trackNamesPersistPath()
	data, err := os.ReadFile(namesPath)
	if err != nil {
		fallbackPath := filepath.Join(dataDir(), "track_names.json")
		data, err = os.ReadFile(fallbackPath)
	}
	if err == nil {
		var names map[string]string
		if err := json.Unmarshal(data, &names); err == nil && len(names) > 0 {
			tracksCatalogMu.Lock()
			for id, title := range names {
				if id != "" && title != "" {
					filename := id
					if !strings.HasSuffix(filename, ".opus") {
						filename = id + ".opus"
					}
					t := model.Track{
						ID:       id,
						Title:    title,
						Filename: filename,
						URL:      "/musicas/" + filename,
					}
					tracksCatalog[id] = t
					base := strings.TrimSuffix(filename, ".opus")
					tracksCatalog[base] = t
					tracksCatalog[filename] = t
				}
			}
			tracksCatalogMu.Unlock()
			log.Printf("%d nomes de músicas restaurados de track_names.json", len(names))
		}
	}

	// 2. Carrega de tracks.json (suporta formato mapa map[string]Track e lista []Track)
	if data, err := os.ReadFile(tracksPersistPath()); err == nil {
		var mapSaved map[string]model.Track
		if err := json.Unmarshal(data, &mapSaved); err == nil && len(mapSaved) > 0 {
			tracksCatalogMu.Lock()
			for _, t := range mapSaved {
				if t.ID != "" {
					tracksCatalog[t.ID] = t
					if t.Filename != "" {
						base := strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
						tracksCatalog[base] = t
						tracksCatalog[t.Filename] = t
					}
				}
			}
			tracksCatalogMu.Unlock()
			log.Printf("%d metadados de músicas restaurados de tracks.json (mapa)", len(mapSaved))
		} else {
			var sliceSaved []model.Track
			if err := json.Unmarshal(data, &sliceSaved); err == nil && len(sliceSaved) > 0 {
				tracksCatalogMu.Lock()
				for _, t := range sliceSaved {
					if t.ID != "" {
						tracksCatalog[t.ID] = t
						if t.Filename != "" {
							base := strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
							tracksCatalog[base] = t
							tracksCatalog[t.Filename] = t
						}
					}
				}
				tracksCatalogMu.Unlock()
				log.Printf("%d metadados de músicas restaurados de tracks.json (lista)", len(sliceSaved))
			}
		}
	}

	// 3. Enriquece/migra dos repositórios de estações ativas
	stationsMu.RLock()
	tracksCatalogMu.Lock()
	migrated := 0
	for _, s := range stations {
		s.RLock()
		for _, t := range s.Repository {
			if t.ID != "" {
				existing, exists := tracksCatalog[t.ID]
				if !exists || existing.Title == "" || strings.HasPrefix(existing.Title, "Música ") {
					tracksCatalog[t.ID] = t
					if t.Filename != "" {
						base := strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
						tracksCatalog[base] = t
						tracksCatalog[t.Filename] = t
					}
					migrated++
				}
			}
		}
		s.RUnlock()
	}
	tracksCatalogMu.Unlock()
	stationsMu.RUnlock()

	if migrated > 0 {
		log.Printf("%d metadados de músicas migrados das estações", migrated)
	}

	go SaveTracksCatalog()
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

		// Atualiza repositório e playlist usando os metadados salvos no catálogo se o título for genérico
		station.Repository = ss.Repository
		for i, t := range station.Repository {
			if t.ID != "" {
				if catTrack, exists := tracksCatalog[t.ID]; exists && catTrack.Title != "" {
					if t.Title == "" || strings.HasPrefix(t.Title, "Música ") {
						station.Repository[i].Title = catTrack.Title
					}
				} else {
					tracksCatalog[t.ID] = t
				}
			}
		}

		station.Playlist = ss.Playlist
		for i, t := range station.Playlist {
			if t.ID != "" {
				if catTrack, exists := tracksCatalog[t.ID]; exists && catTrack.Title != "" {
					if t.Title == "" || strings.HasPrefix(t.Title, "Música ") {
						station.Playlist[i].Title = catTrack.Title
					}
				}
			}
		}

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


