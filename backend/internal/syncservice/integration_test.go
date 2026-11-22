package syncservice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"

	gorillaWS "github.com/gorilla/websocket"
)

// INT-01: PLAY na réplica A chega ao ouvinte na réplica B em menos de 200 ms (ADR-0001).
func TestINT01_PlayFanoutBetweenReplicas(t *testing.T) {
	t.Setenv("JWT_SECRET", "super-segredo-de-teste-longo-com-mais-de-32-chars!")
	broker := NewMemoryBroker()

	storeA := NewMemorySyncStorageWithBroker("replica-A", broker)
	storeB := NewMemorySyncStorageWithBroker("replica-B", broker)

	svcA := NewSyncService(storeA, "replica-A", nil)
	svcB := NewSyncService(storeB, "replica-B", nil)
	defer svcA.Close()
	defer svcB.Close()

	srvA := httptest.NewServer(http.HandlerFunc(svcA.HandleWebSocket))
	srvB := httptest.NewServer(http.HandlerFunc(svcB.HandleWebSocket))
	defer srvA.Close()
	defer srvB.Close()

	const stationID = "INT01A"

	// 1. Ouvinte conecta na Réplica B
	wsUrlB := "ws" + strings.TrimPrefix(srvB.URL, "http") + "/ws/stations/" + stationID
	dialer := gorillaWS.Dialer{}
	connB, _, err := dialer.Dial(wsUrlB, http.Header{
		"Sec-WebSocket-Protocol": {"radiosync.v1"},
	})
	if err != nil {
		t.Fatalf("falha ao conectar ouvinte na réplica B: %v", err)
	}
	defer connB.Close()

	// Lê mensagens iniciais de conexão (STATE_UPDATE e DJ_STATUS)
	connB.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 2; i++ {
		_, _, err := connB.ReadMessage()
		if err != nil {
			t.Fatalf("erro ao ler mensagens iniciais na réplica B: %v", err)
		}
	}

	// 2. DJ conecta na Réplica A com token válido
	djToken, err := auth.GenerateDJToken(stationID)
	if err != nil {
		t.Fatalf("erro ao gerar token de DJ: %v", err)
	}

	wsUrlA := "ws" + strings.TrimPrefix(srvA.URL, "http") + "/ws/stations/" + stationID
	connA, _, err := dialer.Dial(wsUrlA, http.Header{
		"Sec-WebSocket-Protocol": {"radiosync.v1", "radiosync.auth." + djToken},
	})
	if err != nil {
		t.Fatalf("falha ao conectar DJ na réplica A: %v", err)
	}
	defer connA.Close()

	// 3. DJ envia comando PLAY na Réplica A
	t0 := time.Now()
	playCmd, _ := json.Marshal(map[string]interface{}{"type": "PLAY"})
	if err := connA.WriteMessage(gorillaWS.TextMessage, playCmd); err != nil {
		t.Fatalf("erro ao enviar PLAY pelo DJ na réplica A: %v", err)
	}

	// 4. Ouvinte na Réplica B deve receber STATE_UPDATE com isPlaying: true em < 200 ms
	connB.SetReadDeadline(time.Now().Add(2 * time.Second))
	receivedPlay := false
	var latency time.Duration

	for !receivedPlay {
		_, msg, err := connB.ReadMessage()
		if err != nil {
			t.Fatalf("ouvinte na réplica B não recebeu o PLAY: %v", err)
		}

		var event struct {
			Type  string               `json:"type"`
			State *model.PlaybackState `json:"state"`
		}
		if err := json.Unmarshal(msg, &event); err == nil && event.Type == "STATE_UPDATE" {
			if event.State != nil && event.State.IsPlaying {
				latency = time.Since(t0)
				receivedPlay = true
			}
		}
	}

	t.Logf("INT-01 SUCESSO: PLAY propagado da Réplica A para a Réplica B em %v", latency)
	if latency >= 200*time.Millisecond {
		t.Fatalf("critério de sucesso violado: latência %v excede 200 ms", latency)
	}
}

// RES-01: Derrubar réplica e cliente reconectar em outra réplica em menos de 5 s
func TestRES01_ReplicaFailoverReconnection(t *testing.T) {
	t.Setenv("JWT_SECRET", "super-segredo-de-teste-longo-com-mais-de-32-chars!")
	broker := NewMemoryBroker()

	storeA := NewMemorySyncStorageWithBroker("replica-A", broker)
	storeB := NewMemorySyncStorageWithBroker("replica-B", broker)

	// Prepara estado quente no storage compartilhado (música tocando)
	const stationID = "RES01A"
	now := time.Now().UnixMilli()
	_ = storeA.SavePlaybackState(t.Context(), stationID, &model.PlaybackState{
		IsPlaying:   true,
		StartedAt:   now,
		SeekOffset:  30.0,
		CurrentSong: "/musicas/track-resilience.opus",
		Duration:    200.0,
	})

	svcA := NewSyncService(storeA, "replica-A", nil)
	svcB := NewSyncService(storeB, "replica-B", nil)
	defer svcA.Close()
	defer svcB.Close()

	srvA := httptest.NewServer(http.HandlerFunc(svcA.HandleWebSocket))
	srvB := httptest.NewServer(http.HandlerFunc(svcB.HandleWebSocket))
	defer srvB.Close()

	// 1. Cliente conecta inicialmente na Réplica A
	wsUrlA := "ws" + strings.TrimPrefix(srvA.URL, "http") + "/ws/stations/" + stationID
	dialer := gorillaWS.Dialer{}
	conn, _, err := dialer.Dial(wsUrlA, http.Header{"Sec-WebSocket-Protocol": {"radiosync.v1"}})
	if err != nil {
		t.Fatalf("erro ao conectar na réplica A: %v", err)
	}

	// 2. Mata a Réplica A
	tFail := time.Now()
	srvA.Close()
	conn.Close()

	// 3. Simula lógica de reconexão do cliente para a Réplica B
	wsUrlB := "ws" + strings.TrimPrefix(srvB.URL, "http") + "/ws/stations/" + stationID
	connB, _, err := dialer.Dial(wsUrlB, http.Header{"Sec-WebSocket-Protocol": {"radiosync.v1"}})
	if err != nil {
		t.Fatalf("falha ao reconectar na réplica B: %v", err)
	}
	defer connB.Close()

	// 4. Recebe o estado atual na Réplica B
	connB.SetReadDeadline(time.Now().Add(3 * time.Second))
	gotState := false
	for !gotState {
		_, msg, err := connB.ReadMessage()
		if err != nil {
			t.Fatalf("erro ao ler estado da réplica B: %v", err)
		}
		var su model.StateUpdateMsg
		if err := json.Unmarshal(msg, &su); err == nil && su.Type == "STATE_UPDATE" {
			if su.State != nil && su.State.IsPlaying && su.State.CurrentSong == "/musicas/track-resilience.opus" {
				gotState = true
			}
		}
	}

	reconnectDuration := time.Since(tFail)
	t.Logf("RES-01 SUCESSO: failover e recepção do estado atual concluídos em %v", reconnectDuration)
	if reconnectDuration >= 5*time.Second {
		t.Fatalf("critério de sucesso violado: tempo de reconexão %v excede 5 s", reconnectDuration)
	}
}

// RES-04: Reiniciar broker de Pub/Sub e fan-out voltar
func TestRES04_RedisRestart(t *testing.T) {
	broker := NewMemoryBroker()
	storeA := NewMemorySyncStorageWithBroker("replica-A", broker)
	storeB := NewMemorySyncStorageWithBroker("replica-B", broker)

	const stationID = "RES04A"
	recebidos := make(chan string, 10)

	closerB, err := storeB.SubscribeEvents(t.Context(), stationID, func(event *RedisEvent) {
		recebidos <- event.Type
	})
	if err != nil {
		t.Fatalf("erro ao assinar réplica B: %v", err)
	}
	defer closerB.Close()

	// 1. Evento antes do reinício
	_ = storeA.PublishEvent(t.Context(), stationID, &RedisEvent{Type: "PLAY"})
	select {
	case tipo := <-recebidos:
		if tipo != "PLAY" {
			t.Fatalf("esperado PLAY, recebido %s", tipo)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("evento não recebido antes do reinício")
	}

	// 2. Simula "reinício" do broker limpando subscrições
	closerB.Close()

	// 3. Restabelecimento da conexão (re-subscrição)
	newCloserB, err := storeB.SubscribeEvents(t.Context(), stationID, func(event *RedisEvent) {
		recebidos <- event.Type
	})
	if err != nil {
		t.Fatalf("erro ao re-assinar réplica B após reinício: %v", err)
	}
	defer newCloserB.Close()

	// 4. Publica novo evento após o retorno
	_ = storeA.PublishEvent(t.Context(), stationID, &RedisEvent{Type: "PAUSE"})
	select {
	case tipo := <-recebidos:
		if tipo != "PAUSE" {
			t.Fatalf("esperado PAUSE após retorno do broker, recebido %s", tipo)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("fan-out não retornou após reinício do broker!")
	}
	t.Log("RES-04 SUCESSO: fan-out restabelecido após reinício do broker")
}
