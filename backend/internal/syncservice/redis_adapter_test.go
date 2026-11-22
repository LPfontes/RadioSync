package syncservice

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"radio-backend/internal/model"
)

func TestMemorySyncStorage_PubSubFanout(t *testing.T) {
	ctx := context.Background()

	broker := NewMemoryBroker()
	storeA := NewMemorySyncStorageWithBroker("replica-A", broker)
	storeB := NewMemorySyncStorageWithBroker("replica-B", broker)

	recebidoB := make(chan *RedisEvent, 1)
	closerB, err := storeB.SubscribeEvents(ctx, "EST001", func(ev *RedisEvent) {
		recebidoB <- ev
	})
	if err != nil {
		t.Fatalf("erro ao assinar réplica B: %v", err)
	}
	defer closerB.Close()

	payload, _ := json.Marshal(map[string]interface{}{"isPlaying": true})
	err = storeA.PublishEvent(ctx, "EST001", &RedisEvent{
		SenderID:  "replica-A",
		StationID: "EST001",
		Type:      "STATE_UPDATE",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("erro ao publicar evento na réplica A: %v", err)
	}

	select {
	case ev := <-recebidoB:
		if ev.SenderID != "replica-A" {
			t.Errorf("esperado sender replica-A, obtido %s", ev.SenderID)
		}
		if ev.Type != "STATE_UPDATE" {
			t.Errorf("esperado tipo STATE_UPDATE, obtido %s", ev.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("réplica B não recebeu o evento publicado pela réplica A")
	}
}

func TestMemorySyncStorage_GlobalDJLock(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker()
	storeA := NewMemorySyncStorageWithBroker("replica-A", broker)
	storeB := NewMemorySyncStorageWithBroker("replica-B", broker)

	const stationID = "STN123"
	const dj1 = "dj-client-1"
	const dj2 = "dj-client-2"

	// 1. DJ 1 assume a bancada na réplica A
	acquired, err := storeA.AcquireDJLock(ctx, stationID, dj1, 2*time.Second)
	if err != nil || !acquired {
		t.Fatalf("DJ 1 deveria conseguir lock: acquired=%v, err=%v", acquired, err)
	}

	online, err := storeB.IsDJOnline(ctx, stationID)
	if err != nil || !online {
		t.Fatalf("réplica B deveria enxergar DJ online globalmente")
	}

	// 2. DJ 2 tenta assumir na mesma estação via réplica B (regra de 1 DJ por estação)
	acquired2, err := storeB.AcquireDJLock(ctx, stationID, dj2, 2*time.Second)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if acquired2 {
		t.Fatal("DJ 2 na réplica B NÃO deveria conseguir lock enquanto DJ 1 estiver ativo!")
	}

	// 3. Heartbeat do DJ 1 na réplica A
	err = storeA.RefreshDJLock(ctx, stationID, dj1, 2*time.Second)
	if err != nil {
		t.Fatalf("refresh do DJ 1 deveria ter sucesso: %v", err)
	}

	// 4. DJ 1 desconecta na réplica A e libera o lock
	err = storeA.ReleaseDJLock(ctx, stationID, dj1)
	if err != nil {
		t.Fatalf("erro ao liberar lock: %v", err)
	}

	online, _ = storeB.IsDJOnline(ctx, stationID)
	if online {
		t.Fatal("réplica B não deveria mais enxergar DJ online após release")
	}

	// 5. Agora DJ 2 consegue assumir na réplica B
	acquired2, err = storeB.AcquireDJLock(ctx, stationID, dj2, 2*time.Second)
	if err != nil || !acquired2 {
		t.Fatalf("DJ 2 na réplica B deveria conseguir assumir após liberação do DJ 1: acquired=%v", acquired2)
	}
}

func TestMemorySyncStorage_HotState(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker()
	storeA := NewMemorySyncStorageWithBroker("replica-A", broker)
	storeB := NewMemorySyncStorageWithBroker("replica-B", broker)

	const stationID = "STN999"

	// Estado inicial inexistente
	st, err := storeA.GetPlaybackState(ctx, stationID)
	if err != nil {
		t.Fatalf("erro ao ler estado inexistente: %v", err)
	}
	if st != nil {
		t.Fatalf("esperado nil, obtido %v", st)
	}

	// Réplica A salva o estado quente
	now := time.Now().UnixMilli()
	savedState := &model.PlaybackState{
		IsPlaying:   true,
		StartedAt:   now,
		SeekOffset:  15.5,
		CurrentSong: "/musicas/track1.opus",
		Duration:    180.0,
	}

	if err := storeA.SavePlaybackState(ctx, stationID, savedState); err != nil {
		t.Fatalf("erro ao salvar estado: %v", err)
	}

	// Réplica B nova lê o estado quente
	loaded, err := storeB.GetPlaybackState(ctx, stationID)
	if err != nil {
		t.Fatalf("erro ao ler estado quente na réplica B: %v", err)
	}
	if loaded == nil {
		t.Fatal("esperado estado carregado do storage na réplica B")
	}
	if !loaded.IsPlaying || loaded.StartedAt != now || loaded.SeekOffset != 15.5 || loaded.CurrentSong != "/musicas/track1.opus" {
		t.Fatalf("dados do estado quente incorretos: %+v", loaded)
	}
}
