package handler

import (
	"log"
	"net/http"

	"radio-backend/internal/auth"
	"radio-backend/internal/ws"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	gorillaWS "github.com/gorilla/websocket"
)

var upgrader = gorillaWS.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")

	stationsMu.RLock()
	station, ok := stations[stationID]
	stationsMu.RUnlock()

	if !ok {
		http.Error(w, "estação não encontrada", http.StatusNotFound)
		return
	}

	role := "listener"
	token := r.URL.Query().Get("token")
	if token != "" && auth.ValidateDJToken(token, stationID) {
		role = "dj"
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("erro upgrade ws: %v", err)
		return
	}

	client := &ws.Client{
		Hub:  station.Hub,
		ID:   uuid.New().String(),
		Conn: conn,
		Send: make(chan []byte, 256),
		Role: role,
	}

	station.Hub.Register <- client

	go client.WritePump()
	go client.ReadPump()
}
