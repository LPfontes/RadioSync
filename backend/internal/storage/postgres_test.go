package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"radio-backend/internal/model"
)

// TestPostgresRepository_Integration roda contra um banco PostgreSQL real quando
// TEST_DATABASE_URL (ou um Postgres local acessível em DATABASE_URL) estiver disponível,
// e executa a mesma suíte de validação relacional no repositório.
func TestPostgresRepository_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var repo Repository
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}

	if dbURL != "" {
		pgRepo, err := NewPostgresRepository(ctx, dbURL)
		if err != nil {
			t.Logf("Postgres não acessível em %s (%v); executando suíte de paridade no MemoryRepository", dbURL, err)
			repo = NewMemoryRepository()
		} else {
			t.Logf("Conectado ao PostgreSQL real para testes de repositório e migrations")
			repo = pgRepo
		}
	} else {
		repo = NewMemoryRepository()
	}
	defer repo.Close()

	runRepositorySuite(t, ctx, repo)
}

func runRepositorySuite(t *testing.T, ctx context.Context, repo Repository) {
	t.Helper()

	if err := repo.Ping(ctx); err != nil {
		t.Fatalf("Ping falhou: %v", err)
	}

	stID := "TST901"
	_ = repo.DeleteStation(ctx, stID)

	t1 := model.Track{
		ID:       "pg-track-1",
		Title:    "Synthwave Night",
		Filename: "pg-track-1.opus",
		URL:      "/musicas/pg-track-1.opus",
		Duration: 245.5,
		Category: "Eletrônica",
		Theme:    "Noite",
		CoverURL: "https://img.youtube.com/vi/abc/hqdefault.jpg",
	}
	t2 := model.Track{
		ID:       "pg-track-2",
		Title:    "Lo-Fi Chill Beats",
		Filename: "pg-track-2.opus",
		URL:      "/musicas/pg-track-2.opus",
		Duration: 190.0,
	}

	st := &model.Station{
		ID: stID,
		DJ: "initial-dj-token",
		State: &model.PlaybackState{
			IsPlaying:   true,
			StartedAt:   1700000500000,
			SeekOffset:  30.0,
			CurrentSong: t1.URL,
			Duration:    t1.Duration,
		},
		Repository: []model.Track{t1},
		Playlist:   []model.Track{t1},
		Suggestions: []model.SongSuggestion{
			{
				ID:          "sug-pg-1",
				TrackID:     t1.ID,
				Title:       t1.Title,
				SuggestedBy: "Maria",
				CreatedAt:   1700000500123,
				Status:      "pending",
			},
		},
		Mixer: []model.MixerChannel{
			{ID: "fireplace", Name: "Lareira", Type: "preset", PresetKey: "fireplace", Volume: 0.5, IsPlaying: true, Loop: true},
		},
	}

	if err := repo.CreateStation(ctx, st); err != nil {
		t.Fatalf("CreateStation falhou: %v", err)
	}

	// Adiciona 2ª faixa e atualiza metadados
	if err := repo.AddTrackToStation(ctx, stID, t2); err != nil {
		t.Fatalf("AddTrackToStation falhou: %v", err)
	}
	t2Updated := t2
	t2Updated.Title = "Lo-Fi Study Session (Remastered)"
	t2Updated.Category = "Lo-Fi"
	if err := repo.UpdateStationTrack(ctx, stID, t2Updated); err != nil {
		t.Fatalf("UpdateStationTrack falhou: %v", err)
	}

	// Atualiza playlist e DJ token
	if err := repo.ReplacePlaylist(ctx, stID, []model.Track{t2Updated, t1}); err != nil {
		t.Fatalf("ReplacePlaylist falhou: %v", err)
	}
	if err := repo.UpdateStationDJ(ctx, stID, "renewed-dj-token"); err != nil {
		t.Fatalf("UpdateStationDJ falhou: %v", err)
	}

	// Lê estação completa
	got, err := repo.GetStation(ctx, stID)
	if err != nil {
		t.Fatalf("GetStation falhou: %v", err)
	}
	if got.DJ != "renewed-dj-token" {
		t.Errorf("DJ esperado renewed-dj-token, obteve %s", got.DJ)
	}
	if len(got.Repository) != 2 {
		t.Fatalf("Repository esperava 2 faixas, obteve %d", len(got.Repository))
	}
	if len(got.Playlist) != 2 || got.Playlist[0].ID != t2.ID || got.Playlist[0].Title != "Lo-Fi Study Session (Remastered)" {
		t.Errorf("Playlist fora de ordem ou desatualizada: %+v", got.Playlist)
	}
	if len(got.Mixer) != 1 || got.Mixer[0].PresetKey != "fireplace" {
		t.Errorf("Mixer incorreto: %+v", got.Mixer)
	}

	// Cache do YouTube
	if err := repo.SaveYouTubeCache(ctx, "yt_vid_999", t2.ID); err != nil {
		t.Fatalf("SaveYouTubeCache falhou: %v", err)
	}
	ytTrack, ok, err := repo.GetYouTubeCache(ctx, "yt_vid_999")
	if err != nil || !ok || ytTrack.ID != t2.ID {
		t.Errorf("GetYouTubeCache falhou: ok=%v track=%+v err=%v", ok, ytTrack, err)
	}

	// Remove faixa e estação
	if err := repo.RemoveTrackFromStation(ctx, stID, t1.ID); err != nil {
		t.Fatalf("RemoveTrackFromStation falhou: %v", err)
	}
	plAfterRemove, err := repo.GetPlaylist(ctx, stID)
	if err != nil || len(plAfterRemove) != 1 {
		t.Errorf("esperava 1 item na playlist após remover t1, obteve %d", len(plAfterRemove))
	}

	if err := repo.DeleteStation(ctx, stID); err != nil {
		t.Fatalf("DeleteStation falhou: %v", err)
	}
}
