package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"radio-backend/internal/model"
	"radio-backend/internal/storage"
)

var (
	activeRepoMu sync.RWMutex
	activeRepo   storage.Repository = storage.NewMemoryRepository()
)

func init() {
	model.TrackRegisterer = RegisterOrUpdateTrackMetadata
	model.SaveStationsFunc = SaveStations
}

// SetRepository configura o repositório persistente (ex: PostgresRepository) usado pelos handlers.
func SetRepository(repo storage.Repository) {
	if repo == nil {
		return
	}
	activeRepoMu.Lock()
	activeRepo = repo
	activeRepoMu.Unlock()
}

// GetRepositoryBackend retorna o repositório de dados configurado.
func GetRepositoryBackend() storage.Repository {
	activeRepoMu.RLock()
	defer activeRepoMu.RUnlock()
	return activeRepo
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

func RegisterOrUpdateTrackMetadata(t model.Track) {
	if t.ID == "" && t.Filename == "" {
		return
	}
	if t.ID == "" {
		t.ID = strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
	}
	_ = GetRepositoryBackend().UpsertTrack(context.Background(), t)
}

func GetTrackMetadata(trackID string) (model.Track, bool) {
	t, ok, err := GetRepositoryBackend().GetTrack(context.Background(), trackID)
	if err != nil || !ok {
		return model.Track{}, false
	}
	return t, true
}

func GetAllCatalogTracks() []model.Track {
	list, err := GetRepositoryBackend().ListLibraryTracks(context.Background())
	if err != nil {
		return []model.Track{}
	}
	return list
}

// SaveStations sincroniza o estado das estações em memória com o Repository configurado (sem gravar stations.json em disco).
func SaveStations() {
	repo := GetRepositoryBackend()
	stationsMu.RLock()
	snapshots := make([]*model.Station, 0, len(stations))
	for _, s := range stations {
		s.RLock()
		var stCopy model.PlaybackState
		if s.State != nil {
			stCopy = *s.State
		}
		snapshots = append(snapshots, &model.Station{
			ID:          s.ID,
			DJ:          s.DJ,
			State:       &stCopy,
			Repository:  append([]model.Track{}, s.Repository...),
			Playlist:    append([]model.Track{}, s.Playlist...),
			Suggestions: append([]model.SongSuggestion{}, s.Suggestions...),
			Mixer:       append([]model.MixerChannel{}, s.Mixer...),
		})
		s.RUnlock()
	}
	stationsMu.RUnlock()

	ctx := context.Background()
	for _, snap := range snapshots {
		_ = repo.CreateStation(ctx, snap)
	}
}

func SaveTracksCatalog() {
	// No-op: o catálogo é persistido transacionalmente via Repository.UpsertTrack (ADR-0006).
}
