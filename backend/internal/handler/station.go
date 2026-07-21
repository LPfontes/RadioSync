package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"
	"radio-backend/internal/ws"

	"github.com/go-chi/chi/v5"
)

var (
	stations   = make(map[string]*model.Station)
	stationsMu sync.RWMutex
)

var codeRand = rand.New(rand.NewSource(time.Now().UnixNano()))

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generateCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = codeChars[codeRand.Intn(len(codeChars))]
	}
	return string(b)
}

func CreateStation(w http.ResponseWriter, r *http.Request) {
	id := generateCode(6)
	for {
		if _, exists := stations[id]; !exists {
			break
		}
		id = generateCode(6)
	}
	djID := generateCode(12)

	station := model.NewStation(id, djID)

	token, err := auth.GenerateDJToken(id)
	if err != nil {
		http.Error(w, "erro ao gerar token", http.StatusInternalServerError)
		return
	}

	stationsMu.Lock()
	stations[id] = station
	stationsMu.Unlock()

	station.Hub.OnMessage = func(client *ws.Client, msg []byte) {
		station.HandleMessage(client, msg)
		SaveStations()
	}

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"stationId": id,
		"djToken":   token,
	})
}

func GetStation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "stationId")

	stationsMu.RLock()
	station, ok := stations[id]
	stationsMu.RUnlock()

	if !ok {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}

	station.RLock()
	playlist := station.Playlist
	state := station.State
	repo := station.Repository
	station.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         station.ID,
		"playlist":   playlist,
		"state":      state,
		"repository": repo,
	})
}

func GetStationByID(id string) *model.Station {
	stationsMu.RLock()
	defer stationsMu.RUnlock()
	return stations[id]
}
