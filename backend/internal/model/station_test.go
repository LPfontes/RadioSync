package model

import (
	"math"
	"testing"
	"time"
)

// calcPosition é o núcleo da sincronização: todos os ouvintes calculam a
// mesma posição a partir de StartedAt + SeekOffset.
func TestCalcPosition_Pausado(t *testing.T) {
	s := &Station{State: &PlaybackState{IsPlaying: false, SeekOffset: 42.5}}
	if got := s.calcPosition(); got != 42.5 {
		t.Fatalf("pausado: esperado 42.5, obtido %v", got)
	}
}

func TestCalcPosition_Tocando(t *testing.T) {
	startedAt := time.Now().Add(-10 * time.Second).UnixMilli()
	s := &Station{State: &PlaybackState{IsPlaying: true, StartedAt: startedAt, SeekOffset: 5}}

	got := s.calcPosition()
	if math.Abs(got-15) > 0.2 {
		t.Fatalf("tocando: esperado ~15s, obtido %v", got)
	}
}

func TestExtractYouTubeURL(t *testing.T) {
	casos := []struct {
		nome string
		in   SongSuggestion
		want string
	}{
		{"url completa", SongSuggestion{URL: "https://www.youtube.com/watch?v=abc"}, "https://www.youtube.com/watch?v=abc"},
		{"sem esquema", SongSuggestion{URL: "youtu.be/abc"}, "https://youtu.be/abc"},
		{"link no título", SongSuggestion{Title: "toca essa youtube.com/watch?v=xyz por favor"}, "https://youtube.com/watch?v=xyz"},
		{"texto sem link", SongSuggestion{Title: "Bohemian Rhapsody"}, ""},
		{"vazio", SongSuggestion{}, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := ExtractYouTubeURL(c.in); got != c.want {
				t.Fatalf("esperado %q, obtido %q", c.want, got)
			}
		})
	}
}
