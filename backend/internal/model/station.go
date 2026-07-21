package model

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"radio-backend/internal/ws"
)

type PlaybackState struct {
	IsPlaying   bool    `json:"isPlaying"`
	StartedAt   int64   `json:"startedAt"`
	SeekOffset  float64 `json:"seekOffset"`
	CurrentSong string  `json:"currentSong"`
	Duration    float64 `json:"duration"`
}

type Station struct {
	ID         string
	DJ         string
	Hub        *ws.Hub
	State      *PlaybackState
	Repository []Track
	Playlist   []Track
	mu         sync.RWMutex
}

func NewStation(id, dj string) *Station {
	s := &Station{
		ID:         id,
		DJ:         dj,
		State:      &PlaybackState{},
		Repository: make([]Track, 0),
		Playlist:   make([]Track, 0),
	}
	s.Hub = ws.NewHub(id)
	go s.Hub.Run()
	return s
}

func (s *Station) Lock()   { s.mu.Lock() }
func (s *Station) Unlock() { s.mu.Unlock() }
func (s *Station) RLock()   { s.mu.RLock() }
func (s *Station) RUnlock() { s.mu.RUnlock() }

type IncomingMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type StateUpdateMsg struct {
	Type  string         `json:"type"`
	State *PlaybackState `json:"state"`
}

type PlaylistMsg struct {
	Type     string  `json:"type"`
	Playlist []Track `json:"playlist"`
}

func (s *Station) HandleMessage(client *ws.Client, msg []byte) {
	if client.Role != "dj" {
		return
	}

	var incoming IncomingMessage
	if err := json.Unmarshal(msg, &incoming); err != nil {
		log.Printf("erro ao parsear mensagem: %v", err)
		return
	}

	s.Lock()
	defer s.Unlock()

	switch incoming.Type {
	case "PLAY":
		s.State.IsPlaying = true
		s.State.StartedAt = time.Now().UnixMilli()
		s.broadcastState()

	case "PAUSE":
		if s.State.IsPlaying {
			elapsed := time.Now().UnixMilli() - s.State.StartedAt
			s.State.SeekOffset += float64(elapsed) / 1000
			s.State.IsPlaying = false
			s.broadcastState()
		}

	case "SEEK":
		var data struct {
			Position float64 `json:"position"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}
		s.State.SeekOffset = data.Position
		s.State.StartedAt = time.Now().UnixMilli()
		s.broadcastState()

	case "NEXT_TRACK":
		if len(s.Playlist) > 1 {
			s.Playlist = s.Playlist[1:]
			if len(s.Playlist) > 0 {
				track := s.Playlist[0]
				s.State.CurrentSong = track.URL
				s.State.Duration = track.Duration
				s.State.SeekOffset = 0
				s.State.StartedAt = time.Now().UnixMilli()
				s.State.IsPlaying = true
				s.broadcastState()
				s.broadcastPlaylist()
			}
		}

	case "ADD_TO_PLAYLIST":
		var data struct {
			TrackID string `json:"trackId"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}
		for _, t := range s.Repository {
			if t.ID == data.TrackID {
				s.Playlist = append(s.Playlist, t)
				if len(s.Playlist) == 1 {
					s.State.CurrentSong = t.URL
					s.State.Duration = t.Duration
					s.State.SeekOffset = 0
					s.State.StartedAt = time.Now().UnixMilli()
					s.State.IsPlaying = true
				}
				s.broadcastPlaylist()
				if len(s.Playlist) == 1 {
					s.broadcastState()
				}
				return
			}
		}

	case "REMOVE_FROM_PLAYLIST":
		var data struct {
			TrackID string `json:"trackId"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}
		for i, t := range s.Playlist {
			if t.ID == data.TrackID {
				s.Playlist = append(s.Playlist[:i], s.Playlist[i+1:]...)
				s.broadcastPlaylist()
				return
			}
		}
	}
}

func (s *Station) broadcastState() {
	s.Hub.BroadcastJSON(StateUpdateMsg{
		Type:  "STATE_UPDATE",
		State: s.State,
	})
}

func (s *Station) broadcastPlaylist() {
	window := s.Playlist
	if len(window) > 3 {
		window = window[:3]
	}
	s.Hub.BroadcastJSON(PlaylistMsg{
		Type:     "PLAYLIST_UPDATED",
		Playlist: window,
	})
}
