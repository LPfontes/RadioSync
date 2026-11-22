package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"radio-backend/internal/model"
)

func TestMigrationsEmbedded(t *testing.T) {
	upBytes, err := MigrationsFS.ReadFile("migrations/0001_initial_schema.up.sql")
	if err != nil {
		t.Fatalf("esperava ler 0001_initial_schema.up.sql: %v", err)
	}
	upSQL := string(upBytes)
	requiredTables := []string{
		"CREATE TABLE IF NOT EXISTS tracks",
		"CREATE TABLE IF NOT EXISTS stations",
		"CREATE TABLE IF NOT EXISTS station_tracks",
		"CREATE TABLE IF NOT EXISTS playlist_items",
		"CREATE TABLE IF NOT EXISTS suggestions",
		"CREATE TABLE IF NOT EXISTS youtube_cache",
	}
	for _, tbl := range requiredTables {
		if !strings.Contains(upSQL, tbl) {
			t.Errorf("migration up.sql não contém declaração esperada: %s", tbl)
		}
	}

	downBytes, err := MigrationsFS.ReadFile("migrations/0001_initial_schema.down.sql")
	if err != nil {
		t.Fatalf("esperava ler 0001_initial_schema.down.sql: %v", err)
	}
	if !strings.Contains(string(downBytes), "DROP TABLE IF EXISTS stations") {
		t.Errorf("migration down.sql não contém DROP TABLE stations")
	}
}

func TestMemoryObjectStoreContract(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryObjectStore(DefaultMusicBucket)

	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket falhou: %v", err)
	}

	payload := []byte("OggS-fake-opus-payload")
	objectName := "track-123.opus"

	exists, err := store.Exists(ctx, objectName)
	if err != nil || exists {
		t.Fatalf("objeto não deveria existir antes do Put")
	}

	if err := store.Put(ctx, objectName, bytes.NewReader(payload), int64(len(payload)), "audio/ogg"); err != nil {
		t.Fatalf("Put falhou: %v", err)
	}

	exists, err = store.Exists(ctx, objectName)
	if err != nil || !exists {
		t.Fatalf("objeto deveria existir após Put")
	}

	rc, info, err := store.Get(ctx, objectName)
	if err != nil {
		t.Fatalf("Get falhou: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("conteúdo lido difere do gravado")
	}
	if info.Size != int64(len(payload)) || info.ContentType != "audio/ogg" {
		t.Errorf("ObjectInfo inesperado: %+v", info)
	}

	presigned, err := store.PresignedGetURL(ctx, objectName, 10*time.Minute)
	if err != nil || !strings.Contains(presigned, "/musicas/track-123.opus") {
		t.Errorf("PresignedGetURL inválida: %s (err=%v)", presigned, err)
	}

	keys, err := store.ListKeys(ctx, "track-")
	if err != nil || len(keys) != 1 || keys[0] != objectName {
		t.Errorf("ListKeys inesperado: %v (err=%v)", keys, err)
	}

	if err := store.Delete(ctx, objectName); err != nil {
		t.Fatalf("Delete falhou: %v", err)
	}
	exists, _ = store.Exists(ctx, objectName)
	if exists {
		t.Errorf("objeto não deveria existir após Delete")
	}
}

func TestMemoryRepositoryContract(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	defer repo.Close()

	st := &model.Station{
		ID: "ABC234",
		DJ: "jwt-token-1",
		State: &model.PlaybackState{
			IsPlaying:   true,
			StartedAt:   1700000000000,
			SeekOffset:  12.5,
			CurrentSong: "/musicas/t1.opus",
			Duration:    180.0,
		},
		Repository: []model.Track{
			{ID: "t1", Title: "Faixa 1", Filename: "t1.opus", URL: "/musicas/t1.opus", Duration: 180.0},
		},
		Playlist: []model.Track{
			{ID: "t1", Title: "Faixa 1", Filename: "t1.opus", URL: "/musicas/t1.opus", Duration: 180.0},
		},
		Suggestions: []model.SongSuggestion{},
		Mixer:       []model.MixerChannel{{ID: "rain", Name: "Chuva", Volume: 0.4, IsPlaying: true}},
	}

	if err := repo.CreateStation(ctx, st); err != nil {
		t.Fatalf("CreateStation falhou: %v", err)
	}

	loaded, err := repo.GetStation(ctx, "ABC234")
	if err != nil {
		t.Fatalf("GetStation falhou: %v", err)
	}
	if loaded.ID != "ABC234" || !loaded.State.IsPlaying || len(loaded.Playlist) != 1 {
		t.Fatalf("estação carregada incorreta: %+v", loaded)
	}

	// Adiciona faixa 2 e reordena playlist
	t2 := model.Track{ID: "t2", Title: "Faixa 2", Filename: "t2.opus", URL: "/musicas/t2.opus", Duration: 210}
	if err := repo.AddTrackToStation(ctx, "ABC234", t2); err != nil {
		t.Fatalf("AddTrackToStation falhou: %v", err)
	}
	if err := repo.ReplacePlaylist(ctx, "ABC234", []model.Track{t2, st.Repository[0]}); err != nil {
		t.Fatalf("ReplacePlaylist falhou: %v", err)
	}

	pl, err := repo.GetPlaylist(ctx, "ABC234")
	if err != nil || len(pl) != 2 || pl[0].ID != "t2" || pl[1].ID != "t1" {
		t.Fatalf("ordem da playlist incorreta: %+v (err=%v)", pl, err)
	}

	// Sugestões
	sug := model.SongSuggestion{
		ID:          "sug_1",
		TrackID:     "t2",
		Title:       "Faixa 2",
		SuggestedBy: "Luiz",
		CreatedAt:   1700000001000,
		Status:      "pending",
	}
	if err := repo.SaveSuggestion(ctx, "ABC234", sug); err != nil {
		t.Fatalf("SaveSuggestion falhou: %v", err)
	}
	sugs, err := repo.ListSuggestions(ctx, "ABC234")
	if err != nil || len(sugs) != 1 || sugs[0].SuggestedBy != "Luiz" {
		t.Fatalf("ListSuggestions incorreto: %+v", sugs)
	}

	// Cache do YouTube
	if err := repo.SaveYouTubeCache(ctx, "dQw4w9WgXcQ", "t2"); err != nil {
		t.Fatalf("SaveYouTubeCache falhou: %v", err)
	}
	cachedTrack, ok, err := repo.GetYouTubeCache(ctx, "dQw4w9WgXcQ")
	if err != nil || !ok || cachedTrack.ID != "t2" {
		t.Fatalf("GetYouTubeCache falhou: ok=%v track=%+v err=%v", ok, cachedTrack, err)
	}

	// Biblioteca global
	lib, err := repo.ListLibraryTracks(ctx)
	if err != nil || len(lib) != 2 {
		t.Fatalf("ListLibraryTracks esperava 2 faixas, obteve %d", len(lib))
	}
}
