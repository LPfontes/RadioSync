package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"radio-backend/internal/model"
	"radio-backend/internal/security"

	"github.com/go-chi/chi/v5"
	gorillaWS "github.com/gorilla/websocket"
)

// Handshake real do WebSocket: origem estranha recebe 403, mesma origem e
// clientes sem Origin conectam (proteção contra Cross-Site WebSocket Hijacking).
func TestWebSocketOrigin(t *testing.T) {
	const id = "TSTWS1"
	stationsMu.Lock()
	stations[id] = model.NewStation(id, "")
	stationsMu.Unlock()
	t.Cleanup(func() {
		stationsMu.Lock()
		delete(stations, id)
		stationsMu.Unlock()
		SetWebSocketOriginCheck(security.CheckWebSocketOrigin(nil))
	})
	SetWebSocketOriginCheck(security.CheckWebSocketOrigin([]string{"https://radio.exemplo.com"}))

	r := chi.NewRouter()
	r.Get("/ws/stations/{stationId}", HandleWebSocket)
	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/stations/" + id
	host := strings.TrimPrefix(srv.URL, "http://")

	casos := []struct {
		nome, origin string
		status       int
	}{
		{"sem Origin", "", http.StatusSwitchingProtocols},
		{"mesma origem", "http://" + host, http.StatusSwitchingProtocols},
		{"allowlist", "https://radio.exemplo.com", http.StatusSwitchingProtocols},
		{"origem estranha", "https://evil.example", http.StatusForbidden},
	}
	for _, c := range casos {
		hdr := http.Header{}
		if c.origin != "" {
			hdr.Set("Origin", c.origin)
		}
		conn, resp, err := gorillaWS.DefaultDialer.Dial(wsURL, hdr)
		if conn != nil {
			conn.Close()
		}
		if resp == nil {
			t.Fatalf("%s: sem resposta (%v)", c.nome, err)
		}
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, esperado %d", c.nome, resp.StatusCode, c.status)
		}
	}
}
