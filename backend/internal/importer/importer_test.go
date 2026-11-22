package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"radio-backend/internal/storage"
)

func TestImporter_JSONAndOpus_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	musicDir := filepath.Join(tmpDir, "musicas")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(musicDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Cria arquivos .opus locais
	if err := os.WriteFile(filepath.Join(musicDir, "track-alpha.opus"), []byte("OggS-alpha-audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "track-beta.opus"), []byte("OggS-beta-audio"), 0600); err != nil {
		t.Fatal(err)
	}

	// 2. Cria track_names.json
	trackNamesJSON := `{
		"track-alpha": "Alpha Classics",
		"track-beta": "Beta Groove"
	}`
	if err := os.WriteFile(filepath.Join(musicDir, "track_names.json"), []byte(trackNamesJSON), 0600); err != nil {
		t.Fatal(err)
	}

	// 3. Cria yt_cache.json
	ytCacheJSON := `{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": {
			"id": "track-yt-1",
			"title": "Never Gonna Give You Up",
			"filename": "track-yt-1.opus",
			"url": "/musicas/track-yt-1.opus",
			"duration": 213.0
		}
	}`
	if err := os.WriteFile(filepath.Join(dataDir, "yt_cache.json"), []byte(ytCacheJSON), 0600); err != nil {
		t.Fatal(err)
	}

	// 4. Cria stations.json
	stationsJSON := `[
		{
			"id": "IMP123",
			"dj": "token-legado",
			"state": {
				"isPlaying": true,
				"startedAt": 1700001111000,
				"seekOffset": 15.2,
				"currentSong": "/musicas/track-alpha.opus",
				"duration": 180.0
			},
			"repository": [
				{
					"id": "track-alpha",
					"title": "Música track-alpha",
					"filename": "track-alpha.opus",
					"url": "/musicas/track-alpha.opus",
					"duration": 180.0
				},
				{
					"id": "track-beta",
					"title": "Beta Groove",
					"filename": "track-beta.opus",
					"url": "/musicas/track-beta.opus",
					"duration": 200.0
				}
			],
			"playlist": [
				{
					"id": "track-alpha",
					"title": "Música track-alpha",
					"filename": "track-alpha.opus",
					"url": "/musicas/track-alpha.opus",
					"duration": 180.0
				},
				{
					"id": "track-beta",
					"title": "Beta Groove",
					"filename": "track-beta.opus",
					"url": "/musicas/track-beta.opus",
					"duration": 200.0
				}
			],
			"suggestions": [
				{
					"id": "sug_99",
					"trackId": "track-beta",
					"title": "Beta Groove",
					"suggestedBy": "Carlos",
					"createdAt": 1700001112000,
					"status": "pending"
				}
			]
		}
	]`
	if err := os.WriteFile(filepath.Join(dataDir, "stations.json"), []byte(stationsJSON), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	repo := storage.NewMemoryRepository()
	store := storage.NewMemoryObjectStore(storage.DefaultMusicBucket)

	// 1ª Execução do importador
	rep1, err := Run(ctx, Config{DataDir: dataDir, MusicDir: musicDir}, repo, store)
	if err != nil {
		t.Fatalf("1ª execução do importador falhou: %v", err)
	}
	if rep1.AudioFilesUploaded != 2 {
		t.Errorf("esperava 2 arquivos .opus enviados no 1º run, obteve %d", rep1.AudioFilesUploaded)
	}
	if rep1.StationsImported != 1 || rep1.PlaylistItems != 2 || rep1.Suggestions != 1 {
		t.Errorf("relatório inesperado no 1º run: %+v", rep1)
	}

	// Verifica enriquecimento de track_names.json ("Música track-alpha" -> "Alpha Classics")
	st, err := repo.GetStation(ctx, "IMP123")
	if err != nil {
		t.Fatalf("estação IMP123 não encontrada após importação: %v", err)
	}
	if len(st.Playlist) != 2 || st.Playlist[0].Title != "Alpha Classics" {
		t.Errorf("esperava título enriquecido 'Alpha Classics', obteve: %+v", st.Playlist)
	}

	// Verifica cache do YouTube importado
	ytTrack, ok, err := repo.GetYouTubeCache(ctx, "dQw4w9WgXcQ")
	if err != nil || !ok || ytTrack.Title != "Never Gonna Give You Up" {
		t.Errorf("yt_cache não importado corretamente: ok=%v track=%+v err=%v", ok, ytTrack, err)
	}

	// 2ª Execução (Idempotência: nenhum novo upload de áudio e sem duplicar playlist/sugestões)
	rep2, err := Run(ctx, Config{DataDir: dataDir, MusicDir: musicDir}, repo, store)
	if err != nil {
		t.Fatalf("2ª execução do importador falhou: %v", err)
	}
	if rep2.AudioFilesUploaded != 0 {
		t.Errorf("idempotência violada: esperava 0 uploads de áudio no 2º run, obteve %d", rep2.AudioFilesUploaded)
	}

	stAfter, err := repo.GetStation(ctx, "IMP123")
	if err != nil {
		t.Fatal(err)
	}
	if len(stAfter.Playlist) != 2 || len(stAfter.Suggestions) != 1 || len(stAfter.Repository) != 2 {
		t.Errorf("idempotência violada na estação após 2º run: repo=%d pl=%d sugs=%d",
			len(stAfter.Repository), len(stAfter.Playlist), len(stAfter.Suggestions))
	}
}
