package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
	gorillaWS "github.com/gorilla/websocket"
)

func TestWsTokenFromProtocols(t *testing.T) {
	casos := []struct {
		header   []string
		token    string
		offersV1 bool
	}{
		{nil, "", false},
		{[]string{"radiosync.v1"}, "", true},
		{[]string{"radiosync.v1, radiosync.auth.abc.def.ghi"}, "abc.def.ghi", true},
		{[]string{"radiosync.v1", "radiosync.auth.tok"}, "tok", true}, // headers repetidos
		{[]string{"radiosync.auth.tok"}, "tok", false},
		{[]string{"chat, superchat"}, "", false},
		{[]string{"radiosync.v1,radiosync.auth.a,radiosync.auth.b"}, "a", true}, // vale o primeiro
	}
	for _, c := range casos {
		req := httptest.NewRequest(http.MethodGet, "/ws/stations/X", nil)
		for _, h := range c.header {
			req.Header.Add("Sec-WebSocket-Protocol", h)
		}
		token, v1 := wsTokenFromProtocols(req)
		if token != c.token || v1 != c.offersV1 {
			t.Errorf("%q: obtido (%q, %v), esperado (%q, %v)", c.header, token, v1, c.token, c.offersV1)
		}
	}
}

// Handshake real: o token de DJ vale pelo subprotocolo, o servidor responde só
// "radiosync.v1" (sem ecoar o token) e ?token= na URL não dá papel de DJ.
func TestWebSocket_TokenNoSubprotocolo(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	novaEstacao := func(t *testing.T, id string) *model.Station {
		st := model.NewStation(id, "")
		stationsMu.Lock()
		stations[id] = st
		stationsMu.Unlock()
		t.Cleanup(func() {
			stationsMu.Lock()
			delete(stations, id)
			stationsMu.Unlock()
		})
		return st
	}

	r := chi.NewRouter()
	r.Get("/ws/stations/{stationId}", HandleWebSocket)
	srv := httptest.NewServer(r)
	defer srv.Close()
	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/stations/"

	// djConectado espera o registro assíncrono no Hub.
	djConectado := func(st *model.Station) bool {
		for i := 0; i < 50; i++ {
			if st.Hub.HasDJConnected() {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}

	t.Run("token no subprotocolo vira DJ", func(t *testing.T) {
		const id = "WSAUT1"
		st := novaEstacao(t, id)
		tok, _ := auth.GenerateDJToken(id)

		d := gorillaWS.Dialer{Subprotocols: []string{"radiosync.v1", "radiosync.auth." + tok}}
		conn, resp, err := d.Dial(wsBase+id, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		if conn.Subprotocol() != "radiosync.v1" {
			t.Fatalf("subprotocolo escolhido %q, esperado radiosync.v1", conn.Subprotocol())
		}
		for k, vs := range resp.Header {
			for _, v := range vs {
				if strings.Contains(v, tok) {
					t.Fatalf("token ecoado no header de resposta %s", k)
				}
			}
		}
		if !djConectado(st) {
			t.Fatal("conexão com token válido deveria entrar como DJ")
		}
	})

	t.Run("token na query string é ignorado", func(t *testing.T) {
		const id = "WSAUT2"
		st := novaEstacao(t, id)
		tok, _ := auth.GenerateDJToken(id)

		conn, _, err := gorillaWS.DefaultDialer.Dial(wsBase+id+"?role=dj&token="+tok, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		if djConectado(st) {
			t.Fatal("token na URL não pode mais dar papel de DJ")
		}
	})

	t.Run("token inválido entra como ouvinte", func(t *testing.T) {
		const id = "WSAUT3"
		st := novaEstacao(t, id)
		outro, _ := auth.GenerateDJToken("OUTRA1")

		d := gorillaWS.Dialer{Subprotocols: []string{"radiosync.v1", "radiosync.auth." + outro}}
		conn, _, err := d.Dial(wsBase+id, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		if djConectado(st) {
			t.Fatal("token de outra estação não pode virar DJ")
		}
	})

	t.Run("ouvinte sem subprotocolo continua conectando", func(t *testing.T) {
		const id = "WSAUT4"
		novaEstacao(t, id)
		conn, _, err := gorillaWS.DefaultDialer.Dial(wsBase+id, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		conn.Close()
	})
}

// GET /stations/{id} não aceita mais o token pela query string.
func TestGetStation_IgnoraTokenNaQuery(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")
	const id = "WSAUT5"
	stationsMu.Lock()
	stations[id] = model.NewStation(id, "")
	stationsMu.Unlock()
	t.Cleanup(func() {
		stationsMu.Lock()
		delete(stations, id)
		stationsMu.Unlock()
	})

	r := chi.NewRouter()
	r.Get("/api/v1/stations/{stationId}", GetStation)
	tok, _ := auth.GenerateDJToken(id)

	role := func(req *http.Request) string {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var body map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &body)
		s, _ := body["role"].(string)
		return s
	}

	if got := role(httptest.NewRequest(http.MethodGet, "/api/v1/stations/"+id+"?token="+tok, nil)); got != "listener" {
		t.Fatalf("token na query: role %q, esperado listener", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stations/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if got := role(req); got != "dj" {
		t.Fatalf("token no header: role %q, esperado dj", got)
	}
}
