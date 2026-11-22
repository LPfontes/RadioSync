package handler

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"
	"radio-backend/internal/ws"

	"github.com/go-chi/chi/v5"
)

var (
	stations   = make(map[string]*model.Station)
	stationsMu sync.RWMutex
)

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generateCode(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(codeChars)))
	for i := range b {
		num, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = codeChars[0]
		} else {
			b[i] = codeChars[num.Int64()]
		}
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
	token, err := auth.GenerateDJToken(id)
	if err != nil {
		http.Error(w, "erro ao gerar token", http.StatusInternalServerError)
		return
	}

	station := model.NewStation(id, token)

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
	suggestions := station.Suggestions
	mixer := station.Mixer
	station.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	hasDJ := station.Hub != nil && station.Hub.HasDJConnected()
	
	// Token do DJ só via Authorization: Bearer. Query string (?token=) não é
	// aceita porque a URL vai para os logs de acesso (ADR-0005).
	authHeader := r.Header.Get("Authorization")
	reqToken := strings.TrimPrefix(authHeader, "Bearer ")
	if reqToken == authHeader {
		reqToken = ""
	}

	role := "listener"
	if reqToken != "" && auth.ValidateDJToken(reqToken, id) {
		role = "dj"
	}

	resp := map[string]interface{}{
		"id":          station.ID,
		"playlist":    playlist,
		"state":       state,
		"repository":  repo,
		"suggestions": suggestions,
		"mixer":       mixer,
		"hasDJOnline": hasDJ,
		"role":        role,
	}

	// Expiração deslizante: quem já é DJ recebe um token novo a cada vez que
	// abre a sala, então o token só expira após DJ_TOKEN_TTL sem uso.
	if role == "dj" {
		if renewed, err := auth.GenerateDJToken(id); err == nil {
			resp["djToken"] = renewed
		}
		w.Header().Set("Cache-Control", "no-store")
	}

	json.NewEncoder(w).Encode(resp)
}

func ClaimDJRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "stationId")

	stationsMu.RLock()
	station, ok := stations[id]
	stationsMu.RUnlock()

	if !ok {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}

	if station.Hub != nil && station.Hub.HasDJConnected() {
		http.Error(w, "já existe um DJ conectado nesta estação no momento", http.StatusConflict)
		return
	}

	token, err := auth.GenerateDJToken(id)
	if err != nil {
		http.Error(w, "erro ao gerar token", http.StatusInternalServerError)
		return
	}

	station.Lock()
	station.DJ = token
	station.Unlock()

	go SaveStations()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"stationId": id,
		"djToken":   token,
	})
}

func GetStationByID(id string) *model.Station {
	stationsMu.RLock()
	defer stationsMu.RUnlock()
	return stations[id]
}
