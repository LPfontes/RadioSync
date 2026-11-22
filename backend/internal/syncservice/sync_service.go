package syncservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/handler"
	"radio-backend/internal/metrics"
	"radio-backend/internal/model"
	"radio-backend/internal/security"
	"radio-backend/internal/ws"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	gorillaWS "github.com/gorilla/websocket"
)

// SyncHub gerencia as conexões locais de uma estação nesta réplica.
type SyncHub struct {
	StationID    string
	Clients      map[string]*ws.Client
	Broadcast    chan []byte
	Register     chan *ws.Client
	Unregister   chan *ws.Client
	State        *model.PlaybackState
	Mixer        []model.MixerChannel
	Playlist     []model.Track
	Suggestions  []model.SongSuggestion
	mu           sync.RWMutex
	storage      SyncStorage
	instanceID   string
	closer       func()
}

func (h *SyncHub) Lock()    { h.mu.Lock() }
func (h *SyncHub) Unlock()  { h.mu.Unlock() }
func (h *SyncHub) RLock()   { h.mu.RLock() }
func (h *SyncHub) RUnlock() { h.mu.RUnlock() }

func (h *SyncHub) CalcPosition() float64 {
	h.RLock()
	defer h.RUnlock()
	return h.State.CalcPosition()
}

func (h *SyncHub) BroadcastJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("erro marshal broadcast: %v", err)
		return
	}
	h.Broadcast <- data
}

func (h *SyncHub) Run() {
	for {
		select {
		case client, ok := <-h.Register:
			if !ok {
				return
			}
			h.Lock()
			h.Clients[client.ID] = client
			stateCopy := *h.State
			mixerCopy := append([]model.MixerChannel{}, h.Mixer...)
			h.Unlock()

			metrics.ClientConnected(client.Role)

			// 1. Envia estado quente atual para o cliente que acabou de conectar/reconectar
			dataState, _ := json.Marshal(model.StateUpdateMsg{
				Type:  "STATE_UPDATE",
				State: &stateCopy,
			})
			select {
			case client.Send <- dataState:
				metrics.MessageSent("STATE_UPDATE")
			default:
			}

			// 2. Notifica status de DJ global
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			hasDJ, _ := h.storage.IsDJOnline(ctx, h.StationID)
			cancel()

			dataDJ, _ := json.Marshal(map[string]interface{}{
				"type":        "DJ_STATUS",
				"hasDJOnline": hasDJ,
			})
			select {
			case client.Send <- dataDJ:
				metrics.MessageSent("DJ_STATUS")
			default:
			}

			if len(mixerCopy) > 0 {
				dataMixer, _ := json.Marshal(model.MixerMsg{
					Type:     "MIXER_UPDATED",
					Channels: mixerCopy,
				})
				select {
				case client.Send <- dataMixer:
					metrics.MessageSent("MIXER_UPDATED")
				default:
				}
			}

		case client, ok := <-h.Unregister:
			if !ok {
				return
			}
			h.Lock()
			if _, exists := h.Clients[client.ID]; exists {
				delete(h.Clients, client.ID)
				close(client.Send)
				metrics.ClientDisconnected(client.Role)
			}
			h.Unlock()

		case message := <-h.Broadcast:
			h.RLock()
			for _, client := range h.Clients {
				select {
				case client.Send <- message:
					metrics.MessageSent("BROADCAST")
				default:
					close(client.Send)
					delete(h.Clients, client.ID)
					metrics.ClientDisconnected(client.Role)
				}
			}
			h.RUnlock()
		}
	}
}

// ---------------------------------------------------------------- SyncService

type SyncService struct {
	storage        SyncStorage
	instanceID     string
	allowedOrigins []string
	upgrader       gorillaWS.Upgrader
	hubs           map[string]*SyncHub
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewSyncService(storage SyncStorage, instanceID string, allowedOrigins []string) *SyncService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &SyncService{
		storage:        storage,
		instanceID:     instanceID,
		allowedOrigins: allowedOrigins,
		hubs:           make(map[string]*SyncHub),
		ctx:            ctx,
		cancel:         cancel,
	}

	s.upgrader = gorillaWS.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     security.CheckWebSocketOrigin(allowedOrigins),
	}
	return s
}

func (s *SyncService) SetWebSocketOriginCheck(check func(r *http.Request) bool) {
	s.upgrader.CheckOrigin = check
}

func (s *SyncService) GetOrCreateHub(stationID string) *SyncHub {
	s.mu.Lock()
	defer s.mu.Unlock()

	if hub, ok := s.hubs[stationID]; ok {
		return hub
	}

	hub := &SyncHub{
		StationID:   stationID,
		Clients:     make(map[string]*ws.Client),
		Broadcast:   make(chan []byte, 256),
		Register:    make(chan *ws.Client),
		Unregister:  make(chan *ws.Client),
		State:       &model.PlaybackState{},
		Mixer:       make([]model.MixerChannel, 0),
		Playlist:    make([]model.Track, 0),
		Suggestions: make([]model.SongSuggestion, 0),
		storage:     s.storage,
		instanceID:  s.instanceID,
	}

	// Lê estado quente do Redis (ADR-0001)
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	if hotState, err := s.storage.GetPlaybackState(ctx, stationID); err == nil && hotState != nil {
		hub.State = hotState
	}
	cancel()

	// Assina canal Redis Pub/Sub da estação
	subCloser, err := s.storage.SubscribeEvents(s.ctx, stationID, func(ev *RedisEvent) {
		s.handleRemoteEvent(hub, ev)
	})
	if err == nil {
		hub.closer = func() { subCloser.Close() }
	}

	s.hubs[stationID] = hub
	go hub.Run()
	return hub
}

func (s *SyncService) handleRemoteEvent(hub *SyncHub, ev *RedisEvent) {
	switch ev.Type {
	case "STATE_UPDATE":
		var state model.PlaybackState
		if err := json.Unmarshal(ev.Payload, &state); err == nil {
			hub.Lock()
			hub.State = &state
			hub.Unlock()
			hub.BroadcastJSON(model.StateUpdateMsg{
				Type:  "STATE_UPDATE",
				State: &state,
			})
		}
	case "DJ_STATUS":
		hub.Broadcast <- ev.Payload
	case "PLAYLIST_UPDATED":
		var pl []model.Track
		if err := json.Unmarshal(ev.Payload, &pl); err == nil {
			hub.Lock()
			hub.Playlist = pl
			hub.Unlock()
			hub.BroadcastJSON(model.PlaylistMsg{
				Type:     "PLAYLIST_UPDATED",
				Playlist: pl,
			})
		}
	case "MIXER_UPDATED":
		var chs []model.MixerChannel
		if err := json.Unmarshal(ev.Payload, &chs); err == nil {
			hub.Lock()
			hub.Mixer = chs
			hub.Unlock()
			hub.BroadcastJSON(model.MixerMsg{
				Type:     "MIXER_UPDATED",
				Channels: chs,
			})
		}
	case "SUGGESTIONS_UPDATED":
		var sugs []model.SongSuggestion
		if err := json.Unmarshal(ev.Payload, &sugs); err == nil {
			hub.Lock()
			hub.Suggestions = sugs
			hub.Unlock()
			hub.BroadcastJSON(model.SuggestionsMsg{
				Type:        "SUGGESTIONS_UPDATED",
				Suggestions: sugs,
			})
		}
	}
}

func (s *SyncService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")
	if stationID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "ws" && parts[1] == "stations" {
			stationID = parts[2]
		}
	}
	if stationID == "" {
		http.Error(w, "estação não informada", http.StatusBadRequest)
		return
	}

	role := "listener"
	clientID := uuid.New().String()
	token, offersV1 := handler.WSTokenFromProtocols(r)

	if r.URL.Query().Has("token") {
		log.Printf("ws %s: token na query string ignorado (cliente desatualizado)", stationID)
	}

	isDJ := false
	if token != "" && auth.ValidateDJToken(token, stationID) {
		// Tenta adquirir lock global de DJ no Redis com TTL de 15 segundos
		ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
		acquired, err := s.storage.AcquireDJLock(ctx, stationID, clientID, 15*time.Second)
		cancel()

		if err == nil && acquired {
			role = "dj"
			isDJ = true
			log.Printf("cliente %s assumiu como DJ global na estação %s (réplica %s)", clientID, stationID, s.instanceID)
		} else {
			log.Printf("token de DJ fornecido para a estação %s, porém já existe um DJ conectado globalmente. Entrando como ouvinte.", stationID)
		}
	}

	var respHeader http.Header
	if offersV1 {
		respHeader = http.Header{"Sec-Websocket-Protocol": {handler.WSProtocol}}
	}

	conn, err := s.upgrader.Upgrade(w, r, respHeader)
	if err != nil {
		if isDJ {
			_ = s.storage.ReleaseDJLock(context.Background(), stationID, clientID)
		}
		log.Printf("erro upgrade ws: %v", err)
		return
	}

	hub := s.GetOrCreateHub(stationID)

	client := &ws.Client{
		Hub:  &ws.Hub{StationID: stationID},
		ID:   clientID,
		Conn: conn,
		Send: make(chan []byte, 256),
		Role: role,
	}

	hub.Register <- client

	stopHeartbeat := make(chan struct{})
	if isDJ {
		evPayload, _ := json.Marshal(map[string]interface{}{
			"type":        "DJ_STATUS",
			"hasDJOnline": true,
		})
		_ = s.storage.PublishEvent(s.ctx, stationID, &RedisEvent{
			SenderID:  s.instanceID,
			StationID: stationID,
			Type:      "DJ_STATUS",
			Payload:   evPayload,
		})
		hub.Broadcast <- evPayload

		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
					_ = s.storage.RefreshDJLock(ctx, stationID, clientID, 15*time.Second)
					cancel()
				case <-stopHeartbeat:
					return
				case <-s.ctx.Done():
					return
				}
			}
		}()
	}

	go client.WritePump()

	defer func() {
		hub.Unregister <- client
		conn.Close()

		if isDJ {
			close(stopHeartbeat)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.storage.ReleaseDJLock(ctx, stationID, clientID)
			cancel()

			evPayload, _ := json.Marshal(map[string]interface{}{
				"type":        "DJ_STATUS",
				"hasDJOnline": false,
			})
			_ = s.storage.PublishEvent(context.Background(), stationID, &RedisEvent{
				SenderID:  s.instanceID,
				StationID: stationID,
				Type:      "DJ_STATUS",
				Payload:   evPayload,
			})
			hub.Broadcast <- evPayload
			log.Printf("DJ %s desconectou da estação %s", clientID, stationID)
		}
	}()

	conn.SetReadLimit(4096)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		s.handleClientMessage(hub, client, message)
	}
}

func (s *SyncService) handleClientMessage(hub *SyncHub, client *ws.Client, msg []byte) {
	var incoming model.IncomingMessage
	if err := json.Unmarshal(msg, &incoming); err != nil {
		return
	}

	metrics.MessageReceived(incoming.Type)

	// Resposta precisa de sincronização de relógio (ADR-0004)
	if incoming.Type == "SYNC_REQUEST" {
		pos := hub.CalcPosition()
		hub.RLock()
		mixer := append([]model.MixerChannel{}, hub.Mixer...)
		hub.RUnlock()

		data, _ := json.Marshal(map[string]interface{}{
			"type":       "SYNC",
			"position":   pos,
			"mixer":      mixer,
			"serverTime": time.Now().UnixMilli(),
		})
		select {
		case client.Send <- data:
			metrics.MessageSent("SYNC")
		default:
		}
		return
	}

	if incoming.Type == "SUGGEST_TRACK" {
		var data struct {
			TrackID     string `json:"trackId"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			SuggestedBy string `json:"suggestedBy"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}
		if data.SuggestedBy == "" {
			data.SuggestedBy = "Ouvinte Anônimo"
		}
		if data.Title == "" {
			data.Title = "Música Sugerida"
		}

		sug := model.SongSuggestion{
			ID:          fmt.Sprintf("sug_%d", time.Now().UnixNano()),
			TrackID:     data.TrackID,
			Title:       data.Title,
			URL:         data.URL,
			SuggestedBy: data.SuggestedBy,
			CreatedAt:   time.Now().UnixMilli(),
			Status:      "pending",
		}

		hub.Lock()
		hub.Suggestions = append(hub.Suggestions, sug)
		sugsCopy := append([]model.SongSuggestion{}, hub.Suggestions...)
		hub.Unlock()

		msgJSON, _ := json.Marshal(model.SuggestionsMsg{
			Type:        "SUGGESTIONS_UPDATED",
			Suggestions: sugsCopy,
		})
		hub.Broadcast <- msgJSON
		_ = s.storage.PublishEvent(s.ctx, hub.StationID, &RedisEvent{
			SenderID:  s.instanceID,
			StationID: hub.StationID,
			Type:      "SUGGESTIONS_UPDATED",
			Payload:   msgJSON,
		})
		return
	}

	// Comandos privativos do DJ
	if client.Role != "dj" {
		return
	}

	hub.Lock()
	defer hub.Unlock()

	publishState := false
	publishPlaylist := false
	publishMixer := false

	switch incoming.Type {
	case "PLAY":
		hub.State.IsPlaying = true
		hub.State.StartedAt = time.Now().UnixMilli()
		publishState = true

	case "PAUSE":
		if hub.State.IsPlaying {
			elapsed := time.Now().UnixMilli() - hub.State.StartedAt
			hub.State.SeekOffset += float64(elapsed) / 1000
			hub.State.IsPlaying = false
			publishState = true
		}

	case "SEEK":
		var data struct {
			Position float64 `json:"position"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err == nil {
			hub.State.SeekOffset = data.Position
			hub.State.StartedAt = time.Now().UnixMilli()
			publishState = true
		}

	case "NEXT_TRACK":
		if len(hub.Playlist) > 1 {
			hub.Playlist = hub.Playlist[1:]
			track := hub.Playlist[0]
			hub.State.CurrentSong = track.URL
			hub.State.Duration = track.Duration
			hub.State.SeekOffset = 0
			hub.State.StartedAt = time.Now().UnixMilli()
			hub.State.IsPlaying = true
			publishState = true
			publishPlaylist = true
		}

	case "ADD_TO_PLAYLIST":
		var data struct {
			TrackID string       `json:"trackId"`
			Track   *model.Track `json:"track,omitempty"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err == nil && data.Track != nil && data.Track.ID != "" {
			hub.Playlist = append(hub.Playlist, *data.Track)
			if len(hub.Playlist) == 1 {
				hub.State.CurrentSong = data.Track.URL
				hub.State.Duration = data.Track.Duration
				hub.State.SeekOffset = 0
				hub.State.StartedAt = time.Now().UnixMilli()
				hub.State.IsPlaying = true
				publishState = true
			}
			publishPlaylist = true
		}

	case "MIXER_UPDATE":
		var data struct {
			Channels []model.MixerChannel `json:"channels"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err == nil {
			hub.Mixer = data.Channels
			publishMixer = true
		}

	case "MIXER_CLEAR":
		hub.Mixer = make([]model.MixerChannel, 0)
		publishMixer = true
	}

	if publishState {
		_ = s.storage.SavePlaybackState(s.ctx, hub.StationID, hub.State)
		statePayload, _ := json.Marshal(hub.State)
		hub.BroadcastJSON(model.StateUpdateMsg{
			Type:  "STATE_UPDATE",
			State: hub.State,
		})
		_ = s.storage.PublishEvent(s.ctx, hub.StationID, &RedisEvent{
			SenderID:  s.instanceID,
			StationID: hub.StationID,
			Type:      "STATE_UPDATE",
			Payload:   statePayload,
		})
	}

	if publishPlaylist {
		plPayload, _ := json.Marshal(hub.Playlist)
		hub.BroadcastJSON(model.PlaylistMsg{
			Type:     "PLAYLIST_UPDATED",
			Playlist: hub.Playlist,
		})
		_ = s.storage.PublishEvent(s.ctx, hub.StationID, &RedisEvent{
			SenderID:  s.instanceID,
			StationID: hub.StationID,
			Type:      "PLAYLIST_UPDATED",
			Payload:   plPayload,
		})
	}

	if publishMixer {
		mxPayload, _ := json.Marshal(hub.Mixer)
		hub.BroadcastJSON(model.MixerMsg{
			Type:     "MIXER_UPDATED",
			Channels: hub.Mixer,
		})
		_ = s.storage.PublishEvent(s.ctx, hub.StationID, &RedisEvent{
			SenderID:  s.instanceID,
			StationID: hub.StationID,
			Type:      "MIXER_UPDATED",
			Payload:   mxPayload,
		})
	}
}

func (s *SyncService) Close() error {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.hubs {
		if h.closer != nil {
			h.closer()
		}
	}
	return s.storage.Close()
}
