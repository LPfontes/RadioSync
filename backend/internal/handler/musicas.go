package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

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
