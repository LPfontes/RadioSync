package model

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"radio-backend/internal/ws"
)

var TrackResolver func(trackID string) (Track, bool)

type PlaybackState struct {
	IsPlaying   bool    `json:"isPlaying"`
	StartedAt   int64   `json:"startedAt"`
	SeekOffset  float64 `json:"seekOffset"`
	CurrentSong string  `json:"currentSong"`
	Duration    float64 `json:"duration"`
}

type SongSuggestion struct {
	ID          string `json:"id"`
	TrackID     string `json:"trackId,omitempty"`
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	SuggestedBy string `json:"suggestedBy"`
	CreatedAt   int64  `json:"createdAt"`
	Status      string `json:"status"` // "pending", "approved", "rejected"
}

type Station struct {
	ID          string
	DJ          string
	Hub         *ws.Hub
	State       *PlaybackState
	Repository  []Track
	Playlist    []Track
	Suggestions []SongSuggestion
	mu          sync.RWMutex
}

func NewStation(id, dj string) *Station {
	s := &Station{
		ID:          id,
		DJ:          dj,
		State:       &PlaybackState{},
		Repository:  make([]Track, 0),
		Playlist:    make([]Track, 0),
		Suggestions: make([]SongSuggestion, 0),
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

type SuggestionsMsg struct {
	Type        string           `json:"type"`
	Suggestions []SongSuggestion `json:"suggestions"`
}

func (s *Station) HandleMessage(client *ws.Client, msg []byte) {
	var incoming IncomingMessage
	if err := json.Unmarshal(msg, &incoming); err != nil {
		log.Printf("erro ao parsear mensagem: %v", err)
		return
	}

	if incoming.Type == "SYNC_REQUEST" {
		s.RLock()
		pos := s.calcPosition()
		isPlaying := s.State.IsPlaying
		currentSong := s.State.CurrentSong
		s.RUnlock()
		log.Printf("SYNC_REQUEST de %s: pos=%.2f isPlaying=%v song=%s", client.ID, pos, isPlaying, currentSong)
		data, _ := json.Marshal(map[string]interface{}{
			"type":     "SYNC",
			"position": pos,
		})
		select {
		case client.Send <- data:
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
		s.Lock()
		defer s.Unlock()

		if data.SuggestedBy == "" {
			data.SuggestedBy = "Ouvinte Anônimo"
		}
		if data.Title == "" && data.TrackID != "" {
			for _, t := range s.Repository {
				if t.ID == data.TrackID {
					data.Title = t.Title
					data.URL = t.URL
					break
				}
			}
			if data.Title == "" && TrackResolver != nil {
				if t, ok := TrackResolver(data.TrackID); ok {
					data.Title = t.Title
					data.URL = t.URL
				}
			}
		}
		if data.Title == "" {
			data.Title = "Música Sugerida"
		}

		sug := SongSuggestion{
			ID:          fmt.Sprintf("sug_%d", time.Now().UnixNano()),
			TrackID:     data.TrackID,
			Title:       data.Title,
			URL:         data.URL,
			SuggestedBy: data.SuggestedBy,
			CreatedAt:   time.Now().UnixMilli(),
			Status:      "pending",
		}
		s.Suggestions = append(s.Suggestions, sug)
		s.broadcastSuggestions()
		return
	}

	if client.Role != "dj" {
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
			Track   *Track `json:"track,omitempty"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}

		var targetTrack Track
		found := false

		for _, t := range s.Repository {
			if t.ID == data.TrackID {
				targetTrack = t
				found = true
				break
			}
		}

		if !found && data.Track != nil && data.Track.ID != "" {
			targetTrack = *data.Track
			s.Repository = append(s.Repository, targetTrack)
			found = true
		}

		if !found && TrackResolver != nil {
			if t, ok := TrackResolver(data.TrackID); ok {
				targetTrack = t
				s.Repository = append(s.Repository, targetTrack)
				found = true
			}
		}

		if found {
			s.Playlist = append(s.Playlist, targetTrack)
			if len(s.Playlist) == 1 {
				s.State.CurrentSong = targetTrack.URL
				s.State.Duration = targetTrack.Duration
				s.State.SeekOffset = 0
				s.State.StartedAt = time.Now().UnixMilli()
				s.State.IsPlaying = true
			}
			s.broadcastPlaylist()
			if len(s.Playlist) == 1 {
				s.broadcastState()
			}
		}

	case "PLAY_PLAYLIST_TRACK":
		var data struct {
			TrackID string `json:"trackId"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}

		idx := -1
		for i, t := range s.Playlist {
			if t.ID == data.TrackID {
				idx = i
				break
			}
		}

		if idx >= 0 {
			targetTrack := s.Playlist[idx]
			newPlaylist := make([]Track, 0, len(s.Playlist))
			newPlaylist = append(newPlaylist, targetTrack)
			for i, t := range s.Playlist {
				if i != idx {
					newPlaylist = append(newPlaylist, t)
				}
			}
			s.Playlist = newPlaylist

			s.State.CurrentSong = targetTrack.URL
			s.State.Duration = targetTrack.Duration
			s.State.SeekOffset = 0
			s.State.StartedAt = time.Now().UnixMilli()
			s.State.IsPlaying = true

			s.broadcastState()
			s.broadcastPlaylist()
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

	case "APPROVE_SUGGESTION":
		var data struct {
			SuggestionID string `json:"suggestionId"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}
		idx := -1
		for i, sug := range s.Suggestions {
			if sug.ID == data.SuggestionID {
				idx = i
				break
			}
		}
		if idx >= 0 {
			sug := s.Suggestions[idx]
			s.Suggestions = append(s.Suggestions[:idx], s.Suggestions[idx+1:]...)

			var targetTrack Track
			found := false

			if sug.TrackID != "" {
				for _, t := range s.Repository {
					if t.ID == sug.TrackID {
						targetTrack = t
						found = true
						break
					}
				}
				if !found && TrackResolver != nil {
					if t, ok := TrackResolver(sug.TrackID); ok {
						targetTrack = t
						found = true
					}
				}
			}

			if !found {
				targetTrack = Track{
					ID:       sug.ID,
					Title:    sug.Title,
					URL:      sug.URL,
					Filename: "",
					Duration: 0,
				}
			}

			s.Playlist = append(s.Playlist, targetTrack)
			if len(s.Playlist) == 1 {
				s.State.CurrentSong = targetTrack.URL
				s.State.Duration = targetTrack.Duration
				s.State.SeekOffset = 0
				s.State.StartedAt = time.Now().UnixMilli()
				s.State.IsPlaying = true
			}
			s.broadcastPlaylist()
			if len(s.Playlist) == 1 {
				s.broadcastState()
			}
			s.broadcastSuggestions()
		}

	case "REJECT_SUGGESTION":
		var data struct {
			SuggestionID string `json:"suggestionId"`
		}
		if err := json.Unmarshal(incoming.Data, &data); err != nil {
			return
		}
		for i, sug := range s.Suggestions {
			if sug.ID == data.SuggestionID {
				s.Suggestions = append(s.Suggestions[:i], s.Suggestions[i+1:]...)
				s.broadcastSuggestions()
				break
			}
		}

	case "CLEAR_SUGGESTIONS":
		s.Suggestions = make([]SongSuggestion, 0)
		s.broadcastSuggestions()
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
	if len(window) > 50 {
		window = window[:50]
	}
	s.Hub.BroadcastJSON(PlaylistMsg{
		Type:     "PLAYLIST_UPDATED",
		Playlist: window,
	})
}

func (s *Station) broadcastSuggestions() {
	s.Hub.BroadcastJSON(SuggestionsMsg{
		Type:        "SUGGESTIONS_UPDATED",
		Suggestions: s.Suggestions,
	})
}

func (s *Station) calcPosition() float64 {
	if s.State.IsPlaying {
		elapsed := float64(time.Now().UnixMilli()-s.State.StartedAt) / 1000
		return s.State.SeekOffset + elapsed
	}
	return s.State.SeekOffset
}

