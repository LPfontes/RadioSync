package ws

import (
	"testing"
	"time"
)

func novoClienteFake(h *Hub, id, role string) *Client {
	return &Client{Hub: h, ID: id, Role: role, Send: make(chan []byte, 16)}
}

func receber(t *testing.T, c *Client) []byte {
	t.Helper()
	select {
	case m := <-c.Send:
		return m
	case <-time.After(time.Second):
		t.Fatalf("cliente %s não recebeu mensagem", c.ID)
		return nil
	}
}

// Todo ouvinte da sala deve receber o broadcast (fan-out).
func TestHub_BroadcastParaTodos(t *testing.T) {
	h := NewHub("ABC123")
	go h.Run()

	a := novoClienteFake(h, "a", "listener")
	b := novoClienteFake(h, "b", "listener")
	h.Register <- a
	receber(t, a) // DJ_STATUS do registro de a
	h.Register <- b
	receber(t, a) // DJ_STATUS do registro de b
	receber(t, b)

	h.Broadcast <- []byte(`{"type":"PLAY"}`)
	for _, c := range []*Client{a, b} {
		if got := string(receber(t, c)); got != `{"type":"PLAY"}` {
			t.Fatalf("cliente %s recebeu %q", c.ID, got)
		}
	}
}

func TestHub_DetectaDJ(t *testing.T) {
	h := NewHub("ABC123")
	go h.Run()

	ouvinte := novoClienteFake(h, "o", "listener")
	h.Register <- ouvinte
	receber(t, ouvinte)
	if h.HasDJConnected() {
		t.Fatal("não há DJ conectado ainda")
	}

	dj := novoClienteFake(h, "dj", "dj")
	h.Register <- dj
	receber(t, ouvinte)
	if !h.HasDJConnected() {
		t.Fatal("DJ deveria constar como conectado")
	}
	if h.ClientCount() != 2 {
		t.Fatalf("esperado 2 clientes, obtido %d", h.ClientCount())
	}
}
