package handler

import (
	"encoding/json"
	"net/http"
	"os"
)

func DebugHandler(w http.ResponseWriter, r *http.Request) {
	stationsMu.RLock()
	count := len(stations)
	ids := make([]string, 0, count)
	for id := range stations {
		ids = append(ids, id)
	}
	stationsMu.RUnlock()

	persistFile := persistPath()
	fileInfo, _ := os.Stat(persistFile)
	fileSize := int64(0)
	fileExists := false
	if fileInfo != nil {
		fileSize = fileInfo.Size()
		fileExists = true
	}

	dataDir := dataDir()
	dataEntries, _ := os.ReadDir(dataDir)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stations_loaded": count,
		"station_ids":     ids,
		"data_dir":        dataDir,
		"persist_file": map[string]interface{}{
			"path":   persistFile,
			"exists": fileExists,
			"size":   fileSize,
		},
		"data_dir_contents": func() []string {
			var names []string
			for _, e := range dataEntries {
				names = append(names, e.Name())
			}
			return names
		}(),
		"music_dir": getMusicDir(),
	})
}
