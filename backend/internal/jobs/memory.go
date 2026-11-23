package jobs

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// MemoryStore implementa Store em memória, para testes.
type MemoryStore struct {
	mu        sync.Mutex
	records   map[string]Record
	locks     map[string]*memoryLock
	available bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]Record), locks: make(map[string]*memoryLock), available: true}
}

var errStoreDown = errors.New("redis indisponível")

// SetAvailable simula o Redis fora do ar.
func (s *MemoryStore) SetAvailable(v bool) {
	s.mu.Lock()
	s.available = v
	s.mu.Unlock()
}

func (s *MemoryStore) Save(ctx context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return errStoreDown
	}
	s.records[rec.ID] = rec
	return nil
}

func (s *MemoryStore) Delete(ctx context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return errStoreDown
	}
	delete(s.records, rec.ID)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, jobID string) (Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Record{}, false, errStoreDown
	}
	rec, ok := s.records[jobID]
	return rec, ok, nil
}

func (s *MemoryStore) ListByStation(ctx context.Context, stationID string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, errStoreDown
	}
	list := make([]Record, 0)
	for _, rec := range s.records {
		if rec.StationID == stationID {
			list = append(list, rec)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
	if len(list) > maxPerStation {
		list = list[:maxPerStation]
	}
	return list, nil
}

func (s *MemoryStore) Lock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, false, errStoreDown
	}
	if cur, ok := s.locks[key]; ok && time.Now().Before(cur.expires) {
		return nil, false, nil
	}
	l := &memoryLock{store: s, key: key, expires: time.Now().Add(ttl)}
	s.locks[key] = l
	return l, true, nil
}

type memoryLock struct {
	store   *MemoryStore
	key     string
	expires time.Time
}

func (l *memoryLock) Refresh(ctx context.Context, ttl time.Duration) (bool, error) {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if !l.store.available {
		return false, errStoreDown
	}
	if l.store.locks[l.key] != l || time.Now().After(l.expires) {
		return false, nil
	}
	l.expires = time.Now().Add(ttl)
	return true, nil
}

func (l *memoryLock) Release(ctx context.Context) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.store.locks[l.key] == l {
		delete(l.store.locks, l.key)
	}
	return nil
}
