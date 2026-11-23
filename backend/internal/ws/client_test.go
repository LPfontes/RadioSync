package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// servidorWS sobe um servidor de teste que promove cada conexão a um Client
// registrado no hub, com ReadPump e WritePump rodando como no serviço real.
func servidorWS(t *testing.T, h *Hub, role string) (*httptest.Server, <-chan *Client) {
	t.Helper()
	conectados := make(chan *Client, 4)
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &Client{Hub: h, ID: r.URL.Query().Get("id"), Conn: conn, Send: make(chan []byte, 16), Role: role}
		h.Register <- c
		conectados <- c
		go c.WritePump()
		c.ReadPump() // bloqueia até a conexão cair e então desregistra o cliente
	}))
	t.Cleanup(srv.Close)
	return srv, conectados
}

func discar(t *testing.T, srv *httptest.Server, id string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?id=" + id
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("falha ao conectar: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func lerTexto(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("falha ao ler: %v", err)
	}
	return string(msg)
}

func esperarAte(t *testing.T, descricao string, cond func() bool) {
	t.Helper()
	prazo := time.Now().Add(2 * time.Second)
	for time.Now().Before(prazo) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condição não atingida: %s", descricao)
}

// O que o cliente envia pela conexão chega ao hub (OnMessage) com o cliente
// de origem, que é o que permite ao serviço saber quem mandou.
func TestClient_ReadPumpEntregaMensagemAoHub(t *testing.T) {
	h := NewHub("SALA")
	recebidas := make(chan string, 4)
	h.OnMessage = func(c *Client, msg []byte) { recebidas <- c.ID + ":" + string(msg) }
	go h.Run()

	srv, _ := servidorWS(t, h, "listener")
	conn := discar(t, srv, "ana")

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"PLAY"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-recebidas:
		if got != `ana:{"type":"PLAY"}` {
			t.Fatalf("OnMessage recebeu %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("o hub não recebeu a mensagem do cliente")
	}
}

// O broadcast do hub chega ao navegador pela WritePump.
func TestClient_WritePumpEntregaBroadcast(t *testing.T) {
	h := NewHub("SALA")
	go h.Run()
	srv, _ := servidorWS(t, h, "listener")
	conn := discar(t, srv, "ana")

	// a primeira mensagem é o DJ_STATUS do próprio registro
	if got := lerTexto(t, conn); !strings.Contains(got, "DJ_STATUS") {
		t.Fatalf("primeira mensagem = %q, esperado DJ_STATUS", got)
	}
	h.Broadcast <- []byte(`{"type":"STATE_UPDATE"}`)
	if got := lerTexto(t, conn); got != `{"type":"STATE_UPDATE"}` {
		t.Fatalf("broadcast chegou como %q", got)
	}
}

// Quando a conexão cai, o cliente é removido do hub (sem vazar sala nem DJ).
func TestClient_DesconexaoDesregistraDoHub(t *testing.T) {
	h := NewHub("SALA")
	go h.Run()
	srv, _ := servidorWS(t, h, "dj")
	conn := discar(t, srv, "dj1")

	esperarAte(t, "DJ registrado", h.HasDJConnected)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	conn.Close()

	esperarAte(t, "cliente removido após desconexão", func() bool { return h.ClientCount() == 0 })
	if h.HasDJConnected() {
		t.Fatal("o DJ desconectado ainda consta como online")
	}
}

// Sem OnMessage configurado, mensagens são descartadas sem derrubar o hub.
func TestHub_HandleMessageSemCallbackNaoPanica(t *testing.T) {
	h := NewHub("SALA")
	h.HandleMessage(&Client{ID: "x"}, []byte("qualquer"))
}

// Cliente lento (fila Send cheia) é removido pelo broadcast em vez de travar a sala.
func TestHub_ClienteLentoEDescartado(t *testing.T) {
	h := NewHub("SALA")
	go h.Run()

	lento := &Client{Hub: h, ID: "lento", Role: "listener", Send: make(chan []byte, 1)}
	rapido := novoClienteFake(h, "rapido", "listener")
	h.Register <- lento
	h.Register <- rapido

	// o buffer do lento (1) enche com as confirmações de registro; os próximos
	// broadcasts não cabem e ele deve ser descartado
	for i := 0; i < 5; i++ {
		h.Broadcast <- []byte(`{"type":"X"}`)
	}
	esperarAte(t, "cliente lento removido do hub", func() bool { return h.ClientCount() == 1 })
}
