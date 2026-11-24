package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"radio-backend/internal/ws"
)

// ---------------------------------------------------------------- helpers

// sala monta uma estação com um DJ, um ouvinte registrado no hub (que recebe
// os broadcasts) e devolve também um cliente "mudo" para pedidos pontuais.
type sala struct {
	t       *testing.T
	st      *Station
	dj      *ws.Client
	ouvinte *ws.Client // registrado no hub: recebe os broadcasts
	semHub  *ws.Client // fora do hub: só recebe respostas diretas (SYNC)
}

func novaSala(t *testing.T) *sala {
	t.Helper()
	st := NewStation("SALA01", "dj-token")
	s := &sala{
		t:       t,
		st:      st,
		dj:      &ws.Client{ID: "dj", Role: "dj", Send: make(chan []byte, 64)},
		ouvinte: &ws.Client{ID: "ouv", Role: "listener", Send: make(chan []byte, 64)},
		semHub:  &ws.Client{ID: "solo", Role: "listener", Send: make(chan []byte, 8)},
	}
	st.Hub.Register <- s.ouvinte
	s.esperar("DJ_STATUS") // confirmação do registro
	return s
}

// enviar entrega uma mensagem do cliente à estação, como o WebSocket faria.
func (s *sala) enviar(c *ws.Client, tipo string, dados interface{}) {
	s.t.Helper()
	m := map[string]interface{}{"type": tipo}
	if dados != nil {
		m["data"] = dados
	}
	raw, err := json.Marshal(m)
	if err != nil {
		s.t.Fatal(err)
	}
	s.st.HandleMessage(c, raw)
}

// esperar lê o próximo broadcast do tipo pedido (ignora os de outros tipos) e
// o devolve decodificado.
func (s *sala) esperar(tipo string) map[string]interface{} {
	s.t.Helper()
	prazo := time.After(time.Second)
	for {
		select {
		case raw := <-s.ouvinte.Send:
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				s.t.Fatalf("broadcast inválido: %v", err)
			}
			if m["type"] == tipo {
				return m
			}
		case <-prazo:
			s.t.Fatalf("o ouvinte não recebeu broadcast %s", tipo)
			return nil
		}
	}
}

// semBroadcast garante que NADA do tipo foi transmitido (dá um tempo ao hub).
func (s *sala) semBroadcast(tipo string) {
	s.t.Helper()
	prazo := time.After(150 * time.Millisecond)
	for {
		select {
		case raw := <-s.ouvinte.Send:
			var m map[string]interface{}
			_ = json.Unmarshal(raw, &m)
			if m["type"] == tipo {
				s.t.Fatalf("broadcast %s não deveria ter ocorrido: %s", tipo, raw)
			}
		case <-prazo:
			return
		}
	}
}

func (s *sala) estado() PlaybackState {
	s.st.RLock()
	defer s.st.RUnlock()
	return *s.st.State
}

func (s *sala) playlist() []Track {
	s.st.RLock()
	defer s.st.RUnlock()
	return append([]Track(nil), s.st.Playlist...)
}

func faixa(id string) Track {
	return Track{ID: id, Title: "Faixa " + id, Filename: id + ".opus", URL: "/musicas/" + id + ".opus", Duration: 180}
}

func ids(faixas []Track) string {
	var p []string
	for _, f := range faixas {
		p = append(p, f.ID)
	}
	return strings.Join(p, ",")
}

// ---------------------------------------------------------------- NewStation

func TestNewStation_EstadoInicial(t *testing.T) {
	st := NewStation("ABC", "tok")
	if st.ID != "ABC" || st.DJ != "tok" {
		t.Errorf("id/dj = %q/%q", st.ID, st.DJ)
	}
	if st.State == nil || st.State.IsPlaying {
		t.Errorf("a estação deve começar parada: %+v", st.State)
	}
	if st.Hub == nil || st.Hub.StationID != "ABC" {
		t.Error("hub não foi criado com o id da estação")
	}
	if st.Playlist == nil || st.Repository == nil || st.Suggestions == nil || st.Mixer == nil {
		t.Error("as listas devem começar vazias, não nil (o JSON precisa de [] e não null)")
	}
}

// ---------------------------------------------------------------- permissões

func TestHandleMessage_OuvinteNaoControlaReproducao(t *testing.T) {
	s := novaSala(t)
	for _, tipo := range []string{"PLAY", "PAUSE", "SEEK", "NEXT_TRACK", "MIXER_CLEAR", "CLEAR_SUGGESTIONS"} {
		s.enviar(s.ouvinte, tipo, map[string]float64{"position": 99})
	}
	if e := s.estado(); e.IsPlaying || e.SeekOffset != 0 {
		t.Fatalf("ouvinte alterou o estado: %+v", e)
	}
	s.semBroadcast("STATE_UPDATE")
}

func TestHandleMessage_JSONInvalidoEIgnorado(t *testing.T) {
	s := novaSala(t)
	s.st.HandleMessage(s.dj, []byte(`{isso não é json`))
	s.st.HandleMessage(s.dj, []byte(`{"type":"TIPO_QUE_NAO_EXISTE"}`))
	if e := s.estado(); e.IsPlaying {
		t.Fatalf("mensagem inválida mudou o estado: %+v", e)
	}
}

// ---------------------------------------------------------------- SYNC_REQUEST

func TestHandleMessage_SyncRequestResponde(t *testing.T) {
	s := newSalaParada(t, 12.5)
	s.enviar(s.semHub, "SYNC_REQUEST", nil)

	select {
	case raw := <-s.semHub.Send:
		var m struct {
			Type     string  `json:"type"`
			Position float64 `json:"position"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if m.Type != "SYNC" || m.Position != 12.5 {
			t.Fatalf("resposta = %+v, quer SYNC na posição 12.5", m)
		}
	case <-time.After(time.Second):
		t.Fatal("SYNC_REQUEST não foi respondido")
	}
}

func newSalaParada(t *testing.T, offset float64) *sala {
	s := novaSala(t)
	s.st.Lock()
	s.st.State.SeekOffset = offset
	s.st.Unlock()
	return s
}

// ---------------------------------------------------------------- PLAY / PAUSE / SEEK

func TestHandleMessage_PlayPauseSeek(t *testing.T) {
	s := novaSala(t)

	s.enviar(s.dj, "PLAY", nil)
	m := s.esperar("STATE_UPDATE")
	if m["state"].(map[string]interface{})["isPlaying"] != true {
		t.Fatalf("PLAY não transmitiu isPlaying=true: %v", m)
	}
	if e := s.estado(); !e.IsPlaying || e.StartedAt == 0 {
		t.Fatalf("estado após PLAY: %+v", e)
	}

	// PAUSE soma o tempo tocado ao offset, para retomar do mesmo ponto.
	s.st.Lock()
	s.st.State.StartedAt = time.Now().Add(-5 * time.Second).UnixMilli()
	s.st.State.SeekOffset = 10
	s.st.Unlock()
	s.enviar(s.dj, "PAUSE", nil)
	s.esperar("STATE_UPDATE")
	e := s.estado()
	if e.IsPlaying {
		t.Fatal("PAUSE deveria parar a reprodução")
	}
	if e.SeekOffset < 14.9 || e.SeekOffset > 15.5 {
		t.Fatalf("offset após PAUSE = %v, esperado ~15 (10 + 5 s tocados)", e.SeekOffset)
	}

	s.enviar(s.dj, "SEEK", map[string]float64{"position": 42})
	s.esperar("STATE_UPDATE")
	if e := s.estado(); e.SeekOffset != 42 {
		t.Fatalf("SEEK: offset = %v, quer 42", e.SeekOffset)
	}
}

func TestHandleMessage_PauseSemTocarNaoFazNada(t *testing.T) {
	s := novaSala(t)
	s.enviar(s.dj, "PAUSE", nil)
	s.semBroadcast("STATE_UPDATE")
}

func TestHandleMessage_SeekComDadosInvalidosEIgnorado(t *testing.T) {
	s := novaSala(t)
	s.enviar(s.dj, "SEEK", "texto-no-lugar-de-objeto")
	s.semBroadcast("STATE_UPDATE")
	if e := s.estado(); e.SeekOffset != 0 {
		t.Fatalf("SEEK inválido mudou o offset: %v", e.SeekOffset)
	}
}

// ---------------------------------------------------------------- playlist

func TestHandleMessage_AddToPlaylist(t *testing.T) {
	s := novaSala(t)
	a, b := faixa("a"), faixa("b")
	s.st.Lock()
	s.st.Repository = []Track{a, b}
	s.st.Unlock()

	// A primeira faixa da playlist começa a tocar sozinha.
	s.enviar(s.dj, "ADD_TO_PLAYLIST", map[string]string{"trackId": "a"})
	s.esperar("PLAYLIST_UPDATED")
	s.esperar("STATE_UPDATE")
	if e := s.estado(); !e.IsPlaying || e.CurrentSong != a.URL || e.Duration != 180 {
		t.Fatalf("estado após a 1ª faixa: %+v", e)
	}

	// A segunda só entra na fila.
	s.enviar(s.dj, "ADD_TO_PLAYLIST", map[string]string{"trackId": "b"})
	s.esperar("PLAYLIST_UPDATED")
	if got := ids(s.playlist()); got != "a,b" {
		t.Fatalf("playlist = %s, quer a,b", got)
	}
	if e := s.estado(); e.CurrentSong != a.URL {
		t.Fatalf("a 2ª faixa não deveria trocar a música atual: %+v", e)
	}
}

func TestHandleMessage_AddToPlaylist_FaixaDesconhecidaEIgnorada(t *testing.T) {
	s := novaSala(t)
	s.enviar(s.dj, "ADD_TO_PLAYLIST", map[string]string{"trackId": "nao-existe"})
	s.semBroadcast("PLAYLIST_UPDATED")
	if n := len(s.playlist()); n != 0 {
		t.Fatalf("playlist com %d faixas, quer 0", n)
	}
}

func TestHandleMessage_AddToPlaylist_FaixaInlineEntraNoRepositorio(t *testing.T) {
	s := novaSala(t)
	nova := faixa("inline")
	s.enviar(s.dj, "ADD_TO_PLAYLIST", map[string]interface{}{"trackId": "inline", "track": nova})
	s.esperar("PLAYLIST_UPDATED")

	s.st.RLock()
	defer s.st.RUnlock()
	if len(s.st.Repository) != 1 || s.st.Repository[0].ID != "inline" {
		t.Fatalf("repositório = %+v, a faixa inline deveria ter sido registrada", s.st.Repository)
	}
}

func TestHandleMessage_AddToPlaylist_UsaTrackResolver(t *testing.T) {
	s := novaSala(t)
	TrackResolver = func(id string) (Track, bool) {
		if id == "global" {
			return faixa("global"), true
		}
		return Track{}, false
	}
	defer func() { TrackResolver = nil }()

	s.enviar(s.dj, "ADD_TO_PLAYLIST", map[string]string{"trackId": "global"})
	s.esperar("PLAYLIST_UPDATED")
	if got := ids(s.playlist()); got != "global" {
		t.Fatalf("playlist = %s, quer global (vinda da biblioteca global)", got)
	}
}

func TestHandleMessage_NextTrack(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	s.st.Playlist = []Track{faixa("a"), faixa("b"), faixa("c")}
	s.st.Unlock()

	s.enviar(s.dj, "NEXT_TRACK", nil)
	s.esperar("STATE_UPDATE")
	s.esperar("PLAYLIST_UPDATED")
	if got := ids(s.playlist()); got != "b,c" {
		t.Fatalf("playlist = %s, quer b,c", got)
	}
	if e := s.estado(); !e.IsPlaying || e.CurrentSong != "/musicas/b.opus" || e.SeekOffset != 0 {
		t.Fatalf("estado após NEXT_TRACK: %+v", e)
	}
}

func TestHandleMessage_NextTrack_UltimaFaixaNaoAvanca(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	s.st.Playlist = []Track{faixa("a")}
	s.st.Unlock()
	s.enviar(s.dj, "NEXT_TRACK", nil)
	s.semBroadcast("STATE_UPDATE")
	if got := ids(s.playlist()); got != "a" {
		t.Fatalf("playlist = %s, quer a (não há próxima)", got)
	}
}

func TestHandleMessage_PlayPlaylistTrack(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	s.st.Playlist = []Track{faixa("a"), faixa("b"), faixa("c")}
	s.st.Unlock()

	s.enviar(s.dj, "PLAY_PLAYLIST_TRACK", map[string]string{"trackId": "c"})
	s.esperar("STATE_UPDATE")
	s.esperar("PLAYLIST_UPDATED")
	// a faixa escolhida vai para a frente; a ordem das outras é preservada
	if got := ids(s.playlist()); got != "c,a,b" {
		t.Fatalf("playlist = %s, quer c,a,b", got)
	}
	if e := s.estado(); !e.IsPlaying || e.CurrentSong != "/musicas/c.opus" {
		t.Fatalf("estado: %+v", e)
	}
}

func TestHandleMessage_RemoveFromPlaylist(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	s.st.Playlist = []Track{faixa("a"), faixa("b"), faixa("c")}
	s.st.Unlock()

	s.enviar(s.dj, "REMOVE_FROM_PLAYLIST", map[string]string{"trackId": "b"})
	s.esperar("PLAYLIST_UPDATED")
	if got := ids(s.playlist()); got != "a,c" {
		t.Fatalf("playlist = %s, quer a,c", got)
	}
	// remover algo que não está na lista não transmite nada
	s.enviar(s.dj, "REMOVE_FROM_PLAYLIST", map[string]string{"trackId": "zzz"})
	s.semBroadcast("PLAYLIST_UPDATED")
}

func TestHandleMessage_ReorderPlaylist(t *testing.T) {
	casos := []struct {
		nome     string
		de, para int
		quer     string
		muda     bool
	}{
		{"para o fim", 0, 3, "b,c,d,a", true},
		{"para o início", 3, 0, "d,a,b,c", true},
		{"no meio", 1, 2, "a,c,b,d", true},
		{"mesmo índice", 1, 1, "a,b,c,d", false},
		{"origem negativa", -1, 2, "a,b,c,d", false},
		{"destino fora da lista", 0, 9, "a,b,c,d", false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := novaSala(t)
			s.st.Lock()
			s.st.Playlist = []Track{faixa("a"), faixa("b"), faixa("c"), faixa("d")}
			s.st.Unlock()

			s.enviar(s.dj, "REORDER_PLAYLIST", map[string]int{"fromIndex": c.de, "toIndex": c.para})
			if c.muda {
				s.esperar("PLAYLIST_UPDATED")
			} else {
				s.semBroadcast("PLAYLIST_UPDATED")
			}
			if got := ids(s.playlist()); got != c.quer {
				t.Fatalf("playlist = %s, quer %s", got, c.quer)
			}
		})
	}
}

func TestBroadcastPlaylist_LimitaA50Faixas(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	for i := 0; i < 80; i++ {
		s.st.Playlist = append(s.st.Playlist, faixa(fmt.Sprintf("f%d", i)))
	}
	s.st.Unlock()

	s.enviar(s.dj, "REMOVE_FROM_PLAYLIST", map[string]string{"trackId": "f79"})
	m := s.esperar("PLAYLIST_UPDATED")
	if n := len(m["playlist"].([]interface{})); n != 50 {
		t.Fatalf("broadcast levou %d faixas, o limite é 50", n)
	}
}

// ---------------------------------------------------------------- sugestões

func sugestoes(s *sala) []SongSuggestion {
	s.st.RLock()
	defer s.st.RUnlock()
	return append([]SongSuggestion(nil), s.st.Suggestions...)
}

func TestHandleMessage_SuggestTrack_Padroes(t *testing.T) {
	s := novaSala(t)
	// qualquer ouvinte pode sugerir
	s.enviar(s.ouvinte, "SUGGEST_TRACK", map[string]string{})
	s.esperar("SUGGESTIONS_UPDATED")

	got := sugestoes(s)
	if len(got) != 1 {
		t.Fatalf("%d sugestões, quer 1", len(got))
	}
	if got[0].Title != "Música Sugerida" || got[0].SuggestedBy != "Ouvinte Anônimo" || got[0].Status != "pending" {
		t.Fatalf("sugestão = %+v", got[0])
	}
}

func TestHandleMessage_SuggestTrack_ResolveTituloPeloRepositorio(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	s.st.Repository = []Track{faixa("rep")}
	s.st.Unlock()

	s.enviar(s.ouvinte, "SUGGEST_TRACK", map[string]string{"trackId": "rep", "suggestedBy": "Ana"})
	s.esperar("SUGGESTIONS_UPDATED")
	got := sugestoes(s)[0]
	if got.Title != "Faixa rep" || got.URL != "/musicas/rep.opus" || got.SuggestedBy != "Ana" {
		t.Fatalf("sugestão = %+v", got)
	}
}

func TestHandleMessage_SuggestTrack_ResolveTituloPeloResolver(t *testing.T) {
	s := novaSala(t)
	TrackResolver = func(id string) (Track, bool) { return faixa(id), id == "glob" }
	defer func() { TrackResolver = nil }()

	s.enviar(s.ouvinte, "SUGGEST_TRACK", map[string]string{"trackId": "glob"})
	s.esperar("SUGGESTIONS_UPDATED")
	if got := sugestoes(s)[0]; got.Title != "Faixa glob" {
		t.Fatalf("título = %q, quer o do resolver", got.Title)
	}
}

func TestHandleMessage_AprovarSugestao(t *testing.T) {
	casos := []struct {
		nome       string
		sug        SongSuggestion
		repo       []Track
		quer       string // ids esperados na playlist
		tocaDireto bool
	}{
		{"faixa do repositório por trackId", SongSuggestion{ID: "s1", TrackID: "a", Title: "x"}, []Track{faixa("a")}, "a", true},
		{"faixa do repositório pelo título", SongSuggestion{ID: "s1", Title: "faixa a"}, []Track{faixa("a")}, "a", true},
		{"texto livre vira item de playlist", SongSuggestion{ID: "s1", Title: "Uma música qualquer"}, nil, "s1", true},
		// link do YouTube: o download é um job da Media API (ADR-0002); aprovar
		// só tira da lista de sugestões, não coloca nada na playlist.
		{"link do YouTube não entra direto", SongSuggestion{ID: "s1", Title: "https://youtu.be/abc"}, nil, "", false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := novaSala(t)
			s.st.Lock()
			s.st.Repository = c.repo
			s.st.Suggestions = []SongSuggestion{c.sug}
			s.st.Unlock()

			s.enviar(s.dj, "APPROVE_SUGGESTION", map[string]string{"suggestionId": c.sug.ID})
			s.esperar("SUGGESTIONS_UPDATED")

			if n := len(sugestoes(s)); n != 0 {
				t.Fatalf("a sugestão aprovada deveria sair da lista (restam %d)", n)
			}
			if got := ids(s.playlist()); got != c.quer {
				t.Fatalf("playlist = %q, quer %q", got, c.quer)
			}
			if e := s.estado(); e.IsPlaying != c.tocaDireto {
				t.Fatalf("isPlaying = %v, quer %v", e.IsPlaying, c.tocaDireto)
			}
		})
	}
}

func TestHandleMessage_RejeitarELimparSugestoes(t *testing.T) {
	s := novaSala(t)
	s.st.Lock()
	s.st.Suggestions = []SongSuggestion{{ID: "s1", Title: "um"}, {ID: "s2", Title: "dois"}, {ID: "s3", Title: "três"}}
	s.st.Unlock()

	s.enviar(s.dj, "REJECT_SUGGESTION", map[string]string{"suggestionId": "s2"})
	s.esperar("SUGGESTIONS_UPDATED")
	if got := sugestoes(s); len(got) != 2 || got[0].ID != "s1" || got[1].ID != "s3" {
		t.Fatalf("sugestões após rejeitar s2: %+v", got)
	}

	s.enviar(s.dj, "CLEAR_SUGGESTIONS", nil)
	s.esperar("SUGGESTIONS_UPDATED")
	if n := len(sugestoes(s)); n != 0 {
		t.Fatalf("restaram %d sugestões após limpar", n)
	}
}

// ---------------------------------------------------------------- mixer

func canal(id string, vol float64) MixerChannel {
	return MixerChannel{ID: id, Name: id, Type: "preset", PresetKey: "rain", Volume: vol, IsPlaying: true}
}

func mixer(s *sala) []MixerChannel {
	s.st.RLock()
	defer s.st.RUnlock()
	return append([]MixerChannel(nil), s.st.Mixer...)
}

func TestHandleMessage_Mixer(t *testing.T) {
	s := novaSala(t)

	s.enviar(s.dj, "MIXER_UPDATE", map[string]interface{}{"channels": []MixerChannel{canal("c1", 0.3), canal("c2", 0.5)}})
	s.esperar("MIXER_UPDATED")
	if n := len(mixer(s)); n != 2 {
		t.Fatalf("mixer com %d canais, quer 2", n)
	}

	// SET_CHANNEL troca um canal existente...
	s.enviar(s.dj, "MIXER_SET_CHANNEL", map[string]interface{}{"channel": canal("c1", 0.9)})
	s.esperar("MIXER_UPDATED")
	if got := mixer(s); len(got) != 2 || got[0].Volume != 0.9 {
		t.Fatalf("canal c1 não foi atualizado: %+v", got)
	}
	// ...e cria o que ainda não existe.
	s.enviar(s.dj, "MIXER_SET_CHANNEL", map[string]interface{}{"channel": canal("c3", 0.1)})
	s.esperar("MIXER_UPDATED")
	if n := len(mixer(s)); n != 3 {
		t.Fatalf("mixer com %d canais, quer 3", n)
	}
	// canal sem id é rejeitado
	s.enviar(s.dj, "MIXER_SET_CHANNEL", map[string]interface{}{"channel": MixerChannel{Name: "sem id"}})
	s.semBroadcast("MIXER_UPDATED")

	s.enviar(s.dj, "MIXER_REMOVE_CHANNEL", map[string]string{"id": "c2"})
	s.esperar("MIXER_UPDATED")
	if got := mixer(s); len(got) != 2 || got[0].ID != "c1" || got[1].ID != "c3" {
		t.Fatalf("mixer após remover c2: %+v", got)
	}

	s.enviar(s.dj, "MIXER_CLEAR", nil)
	m := s.esperar("MIXER_UPDATED")
	if n := len(mixer(s)); n != 0 {
		t.Fatalf("mixer com %d canais após limpar", n)
	}
	// vazio vai como [] e não como null, senão o frontend quebra
	if m["channels"] == nil {
		t.Fatal("MIXER_UPDATED vazio deve levar [] e não null")
	}
}

func TestHandleMessage_MixerUpdateSemCanaisVaiVazio(t *testing.T) {
	s := novaSala(t)
	s.enviar(s.dj, "MIXER_UPDATE", map[string]interface{}{})
	m := s.esperar("MIXER_UPDATED")
	if m["channels"] == nil {
		t.Fatal("channels deve ser [] e não null")
	}
}

// ---------------------------------------------------------------- posição

func TestStation_CalcPositionExportado(t *testing.T) {
	st := &Station{State: &PlaybackState{IsPlaying: true, StartedAt: time.Now().Add(-3 * time.Second).UnixMilli(), SeekOffset: 1}}
	if got := st.CalcPosition(); got < 3.9 || got > 4.5 {
		t.Fatalf("CalcPosition = %v, esperado ~4", got)
	}
}
