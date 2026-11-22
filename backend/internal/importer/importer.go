package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"
	"radio-backend/internal/storage"
)

// Config define os diretórios de origem para a importação única (ADR-0006).
type Config struct {
	DataDir  string // Diretório contendo stations.json, tracks.json, yt_cache.json
	MusicDir string // Diretório contendo arquivos *.opus e track_names.json
}

// Report resume os itens migrados pelo importador único.
type Report struct {
	AudioFilesUploaded int `json:"audioFilesUploaded"`
	TracksImported     int `json:"tracksImported"`
	YouTubeCacheItems  int `json:"youtubeCacheItems"`
	StationsImported   int `json:"stationsImported"`
	PlaylistItems      int `json:"playlistItems"`
	Suggestions        int `json:"suggestions"`
}

type legacySavedStation struct {
	ID          string                 `json:"id"`
	DJ          string                 `json:"dj"`
	State       *model.PlaybackState   `json:"state"`
	Repository  []model.Track          `json:"repository"`
	Playlist    []model.Track          `json:"playlist"`
	Suggestions []model.SongSuggestion `json:"suggestions"`
	Mixer       []model.MixerChannel   `json:"mixer,omitempty"`
}

// Run executa a migração idempotente dos arquivos JSON locais (stations.json,
// track_names.json, tracks.json, yt_cache.json) para o PostgreSQL e dos arquivos
// .opus locais para o bucket "musicas" no MinIO (ADR-0006).
func Run(ctx context.Context, cfg Config, repo storage.Repository, store storage.ObjectStore) (Report, error) {
	var report Report

	if cfg.DataDir == "" {
		cfg.DataDir = os.Getenv("DATA_DIR")
		if cfg.DataDir == "" {
			cfg.DataDir = "./data"
		}
	}
	if cfg.MusicDir == "" {
		cfg.MusicDir = os.Getenv("MUSIC_DIR")
		if cfg.MusicDir == "" {
			cfg.MusicDir = "../musicas"
		}
	}

	if store != nil {
		if err := store.EnsureBucket(ctx); err != nil {
			return report, fmt.Errorf("erro ao garantir bucket no MinIO: %w", err)
		}
	}

	catalog := make(map[string]model.Track)

	// 1. Carrega track_names.json (de MUSIC_DIR ou fallback em DATA_DIR)
	trackNamesPaths := []string{
		filepath.Join(cfg.MusicDir, "track_names.json"),
		filepath.Join(cfg.DataDir, "track_names.json"),
	}
	for _, p := range trackNamesPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var names map[string]string
		if err := json.Unmarshal(data, &names); err == nil {
			for id, title := range names {
				cleanID := strings.TrimSuffix(strings.TrimSpace(id), ".opus")
				title = strings.TrimSpace(title)
				if cleanID == "" || title == "" {
					continue
				}
				filename := cleanID + ".opus"
				catalog[cleanID] = model.Track{
					ID:       cleanID,
					Title:    title,
					Filename: filename,
					URL:      "/musicas/" + filename,
				}
			}
		}
	}

	// 2. Carrega tracks.json (suporta map[string]Track ou []Track)
	if data, err := os.ReadFile(filepath.Join(cfg.DataDir, "tracks.json")); err == nil {
		var mapSaved map[string]model.Track
		if err := json.Unmarshal(data, &mapSaved); err == nil && len(mapSaved) > 0 {
			for k, t := range mapSaved {
				mergeTrackIntoCatalog(catalog, k, t)
			}
		} else {
			var sliceSaved []model.Track
			if err := json.Unmarshal(data, &sliceSaved); err == nil {
				for _, t := range sliceSaved {
					mergeTrackIntoCatalog(catalog, t.ID, t)
				}
			}
		}
	}

	// 3. Carrega yt_cache.json (de DATA_DIR ou MUSIC_DIR)
	ytCacheEntries := make(map[string]string) // videoID -> trackID
	ytPaths := []string{
		filepath.Join(cfg.DataDir, "yt_cache.json"),
		filepath.Join(cfg.MusicDir, "yt_cache.json"),
	}
	for _, p := range ytPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// Formato 1: map[videoKey]model.Track
		var trackMap map[string]model.Track
		if err := json.Unmarshal(data, &trackMap); err == nil && len(trackMap) > 0 {
			validTrackMap := false
			for k, t := range trackMap {
				if t.ID != "" || t.Filename != "" || t.Title != "" {
					validTrackMap = true
					cleanID := mergeTrackIntoCatalog(catalog, t.ID, t)
					if cleanID != "" {
						vid := normalizeVideoKey(k)
						ytCacheEntries[vid] = cleanID
					}
				}
			}
			if validTrackMap {
				continue
			}
		}
		// Formato 2: map[videoKey]string (trackID ou filename)
		var strMap map[string]string
		if err := json.Unmarshal(data, &strMap); err == nil {
			for k, val := range strMap {
				cleanID := strings.TrimSuffix(strings.TrimSpace(val), ".opus")
				if cleanID == "" {
					continue
				}
				if _, exists := catalog[cleanID]; !exists {
					filename := cleanID + ".opus"
					catalog[cleanID] = model.Track{
						ID:       cleanID,
						Title:    "Música " + cleanID,
						Filename: filename,
						URL:      "/musicas/" + filename,
					}
				}
				vid := normalizeVideoKey(k)
				ytCacheEntries[vid] = cleanID
			}
		}
	}

	// 4. Migra arquivos .opus de MUSIC_DIR para o MinIO e garante registro no catálogo
	if entries, err := os.ReadDir(cfg.MusicDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".opus") {
				continue
			}
			filename := e.Name()
			cleanID := strings.TrimSuffix(filename, filepath.Ext(filename))
			fullPath := filepath.Join(cfg.MusicDir, filename)

			if store != nil {
				exists, err := store.Exists(ctx, filename)
				if err != nil {
					return report, fmt.Errorf("erro ao verificar objeto %s no MinIO: %w", filename, err)
				}
				if !exists {
					if err := uploadFileToStore(ctx, store, fullPath, filename); err != nil {
						return report, err
					}
					report.AudioFilesUploaded++
				}
			}

			if _, exists := catalog[cleanID]; !exists {
				catalog[cleanID] = model.Track{
					ID:       cleanID,
					Title:    "Música " + cleanID,
					Filename: filename,
					URL:      "/musicas/" + filename,
				}
			}
		}
	}

	// 5. Persiste todas as faixas do catálogo no Postgres
	for _, t := range catalog {
		if err := repo.UpsertTrack(ctx, t); err != nil {
			return report, fmt.Errorf("erro ao importar faixa %s: %w", t.ID, err)
		}
		report.TracksImported++
	}

	// 6. Persiste entradas do yt_cache.json no Postgres
	for vid, trackID := range ytCacheEntries {
		if err := repo.SaveYouTubeCache(ctx, vid, trackID); err != nil {
			return report, fmt.Errorf("erro ao salvar youtube_cache (%s -> %s): %w", vid, trackID, err)
		}
		report.YouTubeCacheItems++
	}

	// 7. Carrega stations.json e importa estações, repositórios, playlists e sugestões
	stationsPath := filepath.Join(cfg.DataDir, "stations.json")
	if data, err := os.ReadFile(stationsPath); err == nil && len(data) > 0 {
		var saved []legacySavedStation
		if err := json.Unmarshal(data, &saved); err != nil {
			return report, fmt.Errorf("erro ao decodificar stations.json: %w", err)
		}

		for _, ss := range saved {
			if strings.TrimSpace(ss.ID) == "" {
				continue
			}
			djToken := ss.DJ
			if !auth.ValidateDJToken(djToken, ss.ID) {
				if newToken, err := auth.GenerateDJToken(ss.ID); err == nil {
					djToken = newToken
				}
			}
			state := ss.State
			if state == nil {
				state = &model.PlaybackState{}
			}

			// Enriquece títulos das faixas com o catálogo carregado de track_names.json
			repoTracks := make([]model.Track, 0, len(ss.Repository))
			for _, t := range ss.Repository {
				enriched := enrichTrack(t, catalog)
				if enriched.ID != "" {
					if _, inCat := catalog[enriched.ID]; !inCat {
						catalog[enriched.ID] = enriched
						report.TracksImported++
					}
					repoTracks = append(repoTracks, enriched)
				}
			}

			playlistTracks := make([]model.Track, 0, len(ss.Playlist))
			for _, t := range ss.Playlist {
				enriched := enrichTrack(t, catalog)
				if enriched.ID != "" {
					if _, inCat := catalog[enriched.ID]; !inCat {
						catalog[enriched.ID] = enriched
						report.TracksImported++
					}
					playlistTracks = append(playlistTracks, enriched)
				}
			}

			st := &model.Station{
				ID:          ss.ID,
				DJ:          djToken,
				State:       state,
				Repository:  repoTracks,
				Playlist:    playlistTracks,
				Suggestions: ss.Suggestions,
				Mixer:       ss.Mixer,
			}

			if err := repo.CreateStation(ctx, st); err != nil {
				return report, fmt.Errorf("erro ao importar estação %s: %w", ss.ID, err)
			}
			report.StationsImported++
			report.PlaylistItems += len(playlistTracks)
			report.Suggestions += len(ss.Suggestions)
		}
	}

	log.Printf(
		"[importer] Migração concluída: %d estações, %d faixas, %d itens de playlist, %d sugestões, %d cache YT, %d arquivos .opus enviados ao MinIO",
		report.StationsImported,
		report.TracksImported,
		report.PlaylistItems,
		report.Suggestions,
		report.YouTubeCacheItems,
		report.AudioFilesUploaded,
	)
	return report, nil
}

func uploadFileToStore(ctx context.Context, store storage.ObjectStore, fullPath, key string) error {
	f, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo %s: %w", fullPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("erro ao ler stat de %s: %w", fullPath, err)
	}

	if err := store.Put(ctx, key, f, fi.Size(), "audio/ogg"); err != nil {
		return fmt.Errorf("erro ao enviar %s para o MinIO: %w", key, err)
	}
	return nil
}

func mergeTrackIntoCatalog(catalog map[string]model.Track, keyHint string, t model.Track) string {
	cleanID := strings.TrimSuffix(strings.TrimSpace(t.ID), ".opus")
	if cleanID == "" && t.Filename != "" {
		cleanID = strings.TrimSuffix(strings.TrimSpace(t.Filename), filepath.Ext(t.Filename))
	}
	if cleanID == "" && keyHint != "" {
		cleanID = strings.TrimSuffix(strings.TrimSpace(keyHint), ".opus")
	}
	if cleanID == "" {
		return ""
	}
	t.ID = cleanID
	if t.Filename == "" {
		t.Filename = cleanID + ".opus"
	}
	if t.URL == "" {
		t.URL = "/musicas/" + t.Filename
	}
	if existing, ok := catalog[cleanID]; ok {
		if t.Title == "" || (strings.HasPrefix(t.Title, "Música ") && existing.Title != "") {
			t.Title = existing.Title
		}
		if t.Duration <= 0 {
			t.Duration = existing.Duration
		}
		if t.Category == "" {
			t.Category = existing.Category
		}
		if t.Theme == "" {
			t.Theme = existing.Theme
		}
		if t.CoverURL == "" {
			t.CoverURL = existing.CoverURL
		}
	}
	if t.Title == "" {
		t.Title = "Música " + cleanID
	}
	catalog[cleanID] = t
	return cleanID
}

func enrichTrack(t model.Track, catalog map[string]model.Track) model.Track {
	cleanID := strings.TrimSuffix(strings.TrimSpace(t.ID), ".opus")
	if cleanID == "" && t.Filename != "" {
		cleanID = strings.TrimSuffix(strings.TrimSpace(t.Filename), filepath.Ext(t.Filename))
	}
	if cleanID == "" {
		return model.Track{}
	}
	t.ID = cleanID
	if t.Filename == "" {
		t.Filename = cleanID + ".opus"
	}
	if t.URL == "" {
		t.URL = "/musicas/" + t.Filename
	}
	if cat, ok := catalog[cleanID]; ok {
		if t.Title == "" || (strings.HasPrefix(t.Title, "Música ") && cat.Title != "") {
			t.Title = cat.Title
		}
		if t.Duration <= 0 {
			t.Duration = cat.Duration
		}
		if t.Category == "" {
			t.Category = cat.Category
		}
		if t.Theme == "" {
			t.Theme = cat.Theme
		}
		if t.CoverURL == "" {
			t.CoverURL = cat.CoverURL
		}
	}
	if t.Title == "" {
		t.Title = "Música " + cleanID
	}
	return t
}

func normalizeVideoKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "youtube.com") || strings.Contains(raw, "youtu.be") {
		if u, err := url.Parse(raw); err == nil {
			if strings.Contains(u.Host, "youtu.be") {
				if id := strings.TrimPrefix(u.Path, "/"); id != "" {
					return id
				}
			}
			if v := u.Query().Get("v"); v != "" {
				return v
			}
		}
	}
	return raw
}
