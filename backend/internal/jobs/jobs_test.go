package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"radio-backend/internal/model"
	"radio-backend/internal/queue"

	"github.com/google/uuid"
)

// A mesma suíte roda no MemoryStore e, com TEST_REDIS_URL, num Redis real.
func TestMemoryStore(t *testing.T) { runStoreSuite(t, NewMemoryStore()) }

func TestRedisStore(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL não definido")
	}
	s, err := NewRedisStore(url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	runStoreSuite(t, s)
}

func runStoreSuite(t *testing.T, s Store) {
	ctx := context.Background()
	station := "T" + uuid.New().String()[:8]

	t.Run("registro", func(t *testing.T) {
		job := queue.Job{ID: uuid.New().String(), Kind: queue.KindUpload, StationID: station, Title: "Faixa", EnqueuedAt: 1000}
		if _, ok, err := s.Get(ctx, job.ID); ok || err != nil {
			t.Fatalf("não deveria existir: ok=%v err=%v", ok, err)
		}
		rec := NewRecord(job, Queued)
		if err := s.Save(ctx, rec); err != nil {
			t.Fatal(err)
		}
		rec.Status, rec.Track = Ready, &model.Track{ID: "x"}
		if err := s.Save(ctx, rec); err != nil {
			t.Fatal(err)
		}
		got, ok, err := s.Get(ctx, job.ID)
		if err != nil || !ok || got.Status != Ready || got.Title != "Faixa" || got.Track == nil || got.Track.ID != "x" {
			t.Fatalf("obtido %+v ok=%v err=%v", got, ok, err)
		}

		newer := NewRecord(queue.Job{ID: uuid.New().String(), StationID: station, Kind: queue.KindYouTube, VideoURL: "u", EnqueuedAt: 2000}, Queued)
		if err := s.Save(ctx, newer); err != nil {
			t.Fatal(err)
		}
		list, err := s.ListByStation(ctx, station)
		if err != nil || len(list) != 2 || list[0].ID != newer.ID || list[1].ID != job.ID {
			t.Fatalf("lista: %+v err=%v", list, err)
		}
		if list[0].Title != "u" {
			t.Errorf("sem título, deveria mostrar a URL: %q", list[0].Title)
		}
		if err := s.Delete(ctx, newer); err != nil {
			t.Fatal(err)
		}
		if list, _ := s.ListByStation(ctx, station); len(list) != 1 {
			t.Fatalf("depois de apagar: %+v", list)
		}
		if list, err := s.ListByStation(ctx, "outra-"+station); err != nil || list == nil || len(list) != 0 {
			t.Fatalf("outra estação: %v %v", list, err)
		}
	})

	t.Run("lista limitada", func(t *testing.T) {
		st := station + "lim"
		for i := 0; i < maxPerStation+5; i++ {
			_ = s.Save(ctx, NewRecord(queue.Job{ID: uuid.New().String(), StationID: st, EnqueuedAt: int64(i)}, Queued))
		}
		list, err := s.ListByStation(ctx, st)
		if err != nil || len(list) != maxPerStation || list[0].CreatedAt != maxPerStation+4 {
			t.Fatalf("%d itens, err=%v", len(list), err)
		}
	})

	t.Run("lock", func(t *testing.T) {
		key := uuid.New().String()
		a, ok, err := s.Lock(ctx, key, 200*time.Millisecond)
		if err != nil || !ok {
			t.Fatalf("primeiro lock: %v %v", ok, err)
		}
		if _, ok, _ := s.Lock(ctx, key, time.Second); ok {
			t.Fatal("o segundo dono não deveria conseguir")
		}
		for i := 0; i < 3; i++ {
			time.Sleep(120 * time.Millisecond)
			if held, err := a.Refresh(ctx, 200*time.Millisecond); !held || err != nil {
				t.Fatalf("renovação %d: %v %v", i, held, err)
			}
		}
		time.Sleep(300 * time.Millisecond) // o dono parou de renovar
		b, ok, err := s.Lock(ctx, key, time.Second)
		if err != nil || !ok {
			t.Fatalf("o lock expirado deveria estar livre: %v %v", ok, err)
		}
		if held, _ := a.Refresh(ctx, time.Second); held {
			t.Fatal("o dono antigo não pode renovar o lock do novo")
		}
		_ = a.Release(ctx)
		if _, ok, _ := s.Lock(ctx, key, time.Second); ok {
			t.Fatal("o dono antigo não pode soltar o lock do novo")
		}
		_ = b.Release(ctx)
		if _, ok, _ := s.Lock(ctx, key, time.Second); !ok {
			t.Fatal("o lock solto deveria estar livre")
		}
	})
}
