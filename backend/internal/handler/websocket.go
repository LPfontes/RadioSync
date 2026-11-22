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

	// O token de DJ vem no Sec-WebSocket-Protocol, nunca na URL (ver wsauth.go).
	// Um ?token= de clientes antigos é ignorado: a conexão entra como ouvinte.
	role := "listener"
	token, offersV1 := wsTokenFromProtocols(r)
	if r.URL.Query().Has("token") {
		log.Printf("ws %s: token na query string ignorado (cliente desatualizado)", stationID)
	}
	if token != "" && auth.ValidateDJToken(token, stationID) {
		if !station.Hub.HasDJConnected() {
			role = "dj"
		} else {
			log.Printf("token de DJ fornecido para a estação %s, porém já existe um DJ conectado. Entrando como ouvinte.", stationID)
		}
	}

	// Responde só com "radiosync.v1": o subprotocolo com o token não é ecoado.
	var respHeader http.Header
	if offersV1 {
		respHeader = http.Header{"Sec-Websocket-Protocol": {wsProtocol}}
	}

	conn, err := upgrader.Upgrade(w, r, respHeader)
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
