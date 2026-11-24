package syncservice

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"io"
	"log"
	"sync"
	"time"

	"radio-backend/internal/metrics"
	"radio-backend/internal/model"
	"radio-backend/internal/tracing"

	"github.com/redis/go-redis/v9"
)

// RedisEvent representa um evento propagado pelo canal station:{id}:events (ADR-0001).
type RedisEvent struct {
	SenderID  string          `json:"senderId"`
	StationID string          `json:"stationId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	SentAt    int64           `json:"sentAt"`

	// Trace leva o traceparent do comando do DJ até as outras réplicas, para o
	// fan-out aparecer no mesmo trace.
	Trace map[string]string `json:"trace,omitempty"`
}

// SyncStorage define as operações necessárias para fan-out, estado quente e lock de DJ.
type SyncStorage interface {
	PublishEvent(ctx context.Context, stationID string, event *RedisEvent) error
	SubscribeEvents(ctx context.Context, stationID string, onMessage func(event *RedisEvent)) (io.Closer, error)
	SavePlaybackState(ctx context.Context, stationID string, state *model.PlaybackState) error
	GetPlaybackState(ctx context.Context, stationID string) (*model.PlaybackState, error)
	AcquireDJLock(ctx context.Context, stationID string, clientID string, ttl time.Duration) (bool, error)
	RefreshDJLock(ctx context.Context, stationID string, clientID string, ttl time.Duration) error
	ReleaseDJLock(ctx context.Context, stationID string, clientID string) error
	IsDJOnline(ctx context.Context, stationID string) (bool, error)
	Close() error
}

// ---------------------------------------------------------------- RedisSyncStorage

type RedisSyncStorage struct {
	client     *redis.Client
	instanceID string
	closing    chan struct{}
}

func NewRedisSyncStorage(redisURL, instanceID string) (*RedisSyncStorage, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		opts = &redis.Options{Addr: redisURL}
	}
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("falha ao conectar no Redis (%s): %w", redisURL, err)
	}

	return &RedisSyncStorage{
		client:     client,
		instanceID: instanceID,
		closing:    make(chan struct{}),
	}, nil
}

func (s *RedisSyncStorage) channelName(stationID string) string {
	return fmt.Sprintf("station:%s:events", stationID)
}

func (s *RedisSyncStorage) stateKey(stationID string) string {
	return fmt.Sprintf("station:%s:state", stationID)
}

func (s *RedisSyncStorage) djKey(stationID string) string {
	return fmt.Sprintf("station:%s:dj", stationID)
}

func (s *RedisSyncStorage) PublishEvent(ctx context.Context, stationID string, event *RedisEvent) error {
	if event.SenderID == "" {
		event.SenderID = s.instanceID
	}
	if event.StationID == "" {
		event.StationID = stationID
	}
	if event.SentAt == 0 {
		event.SentAt = time.Now().UnixMilli()
	}

	ctx, span := tracing.StartProducer(ctx, "redis.publish "+event.Type,
		attribute.String("messaging.system", "redis"),
		attribute.String("radiosync.station.id", stationID),
		attribute.String("radiosync.event.type", event.Type))
	defer span.End()
	event.Trace = tracing.Inject(ctx)

	data, err := json.Marshal(event)
	if err != nil {
		tracing.Fail(span, err)
		return fmt.Errorf("erro serializar evento Redis: %w", err)
	}

	channel := s.channelName(stationID)
	err = s.client.Publish(ctx, channel, data).Err()
	tracing.Fail(span, err)
	return err
}

type subscriptionCloser struct {
	cancel context.CancelFunc
	pubsub *redis.PubSub
}

func (sc *subscriptionCloser) Close() error {
	sc.cancel()
	return sc.pubsub.Close()
}

// SubscribeEvents assina o canal da estação com suporte a reconexão automática (RES-04).
func (s *RedisSyncStorage) SubscribeEvents(ctx context.Context, stationID string, onMessage func(event *RedisEvent)) (io.Closer, error) {
	channel := s.channelName(stationID)
	subCtx, cancel := context.WithCancel(context.Background())

	pubsub := s.client.Subscribe(subCtx, channel)
	if _, err := pubsub.Receive(ctx); err != nil {
		cancel()
		pubsub.Close()
		return nil, fmt.Errorf("erro ao assinar canal %s: %w", channel, err)
	}

	closer := &subscriptionCloser{
		cancel: cancel,
		pubsub: pubsub,
	}

	go func() {
		defer pubsub.Close()
		backoff := 500 * time.Millisecond

		for {
			select {
			case <-subCtx.Done():
				return
			case <-s.closing:
				return
			default:
			}

			ch := pubsub.Channel()
			reconnectNeeded := false

			for msg := range ch {
				var ev RedisEvent
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
					log.Printf("redis pubsub: erro ao deserializar mensagem: %v", err)
					continue
				}

				if ev.SenderID == s.instanceID {
					continue
				}

				if ev.SentAt > 0 {
					latency := time.Duration(time.Now().UnixMilli()-ev.SentAt) * time.Millisecond
					if latency >= 0 {
						metrics.RecordFanoutLatency(latency)
					}
				}

				// A réplica que recebe fica no trace do comando do DJ.
				_, span := tracing.StartConsumer(context.Background(), "redis.fanout "+ev.Type, ev.Trace,
					attribute.String("messaging.system", "redis"),
					attribute.String("radiosync.station.id", ev.StationID),
					attribute.String("radiosync.event.type", ev.Type))
				onMessage(&ev)
				span.End()
			}

			select {
			case <-subCtx.Done():
				return
			case <-s.closing:
				return
			default:
				reconnectNeeded = true
			}

			if reconnectNeeded {
				log.Printf("redis pubsub canal %s desconectou; tentando reconectar em %v...", channel, backoff)
				time.Sleep(backoff)
				if backoff < 5*time.Second {
					backoff *= 2
				}

				newPubsub := s.client.Subscribe(subCtx, channel)
				if _, err := newPubsub.Receive(subCtx); err == nil {
					pubsub.Close()
					pubsub = newPubsub
					closer.pubsub = newPubsub
					backoff = 500 * time.Millisecond
					log.Printf("redis pubsub canal %s reconectado com sucesso!", channel)
				} else {
					newPubsub.Close()
				}
			}
		}
	}()

	return closer, nil
}

func (s *RedisSyncStorage) SavePlaybackState(ctx context.Context, stationID string, state *model.PlaybackState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.stateKey(stationID), data, 48*time.Hour).Err()
}

func (s *RedisSyncStorage) GetPlaybackState(ctx context.Context, stationID string) (*model.PlaybackState, error) {
	val, err := s.client.Get(ctx, s.stateKey(stationID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var state model.PlaybackState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *RedisSyncStorage) AcquireDJLock(ctx context.Context, stationID string, clientID string, ttl time.Duration) (bool, error) {
	key := s.djKey(stationID)
	val := fmt.Sprintf("%s@%s", clientID, s.instanceID)
	return s.client.SetNX(ctx, key, val, ttl).Result()
}

var refreshDJScript = redis.NewScript(`
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("pexpire", KEYS[1], ARGV[2])
	else
		return 0
	end
`)

func (s *RedisSyncStorage) RefreshDJLock(ctx context.Context, stationID string, clientID string, ttl time.Duration) error {
	key := s.djKey(stationID)
	val := fmt.Sprintf("%s@%s", clientID, s.instanceID)
	res, err := refreshDJScript.Run(ctx, s.client, []string{key}, val, ttl.Milliseconds()).Int()
	if err != nil {
		return err
	}
	if res == 0 {
		return fmt.Errorf("dj lock expirou ou pertence a outro cliente")
	}
	return nil
}

var releaseDJScript = redis.NewScript(`
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("del", KEYS[1])
	else
		return 0
	end
`)

func (s *RedisSyncStorage) ReleaseDJLock(ctx context.Context, stationID string, clientID string) error {
	key := s.djKey(stationID)
	val := fmt.Sprintf("%s@%s", clientID, s.instanceID)
	return releaseDJScript.Run(ctx, s.client, []string{key}, val).Err()
}

func (s *RedisSyncStorage) IsDJOnline(ctx context.Context, stationID string) (bool, error) {
	key := s.djKey(stationID)
	n, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *RedisSyncStorage) Close() error {
	close(s.closing)
	return s.client.Close()
}

// ---------------------------------------------------------------- MemorySyncStorage (para testes)

type MemorySyncStorage struct {
	mu          sync.RWMutex
	instanceID  string
	subscribers map[string][]func(event *RedisEvent)
	states      map[string]*model.PlaybackState
	djLocks     map[string]djLockInfo
	broker      *MemoryBroker
}

type subscriberInfo struct {
	instanceID string
	callback   func(event *RedisEvent)
}

// MemoryBroker simula a rede Pub/Sub do Redis para testes locais multi-réplicas
type MemoryBroker struct {
	mu          sync.RWMutex
	subscribers map[string][]subscriberInfo
	states      map[string]*model.PlaybackState
	djLocks     map[string]djLockInfo
}

func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{
		subscribers: make(map[string][]subscriberInfo),
		states:      make(map[string]*model.PlaybackState),
		djLocks:     make(map[string]djLockInfo),
	}
}

type djLockInfo struct {
	val       string
	expiresAt time.Time
}

func NewMemorySyncStorage(instanceID string) *MemorySyncStorage {
	return &MemorySyncStorage{
		instanceID: instanceID,
		broker:     NewMemoryBroker(),
	}
}

func NewMemorySyncStorageWithBroker(instanceID string, broker *MemoryBroker) *MemorySyncStorage {
	return &MemorySyncStorage{
		instanceID: instanceID,
		broker:     broker,
	}
}

func (m *MemorySyncStorage) PublishEvent(ctx context.Context, stationID string, event *RedisEvent) error {
	if event.SenderID == "" {
		event.SenderID = m.instanceID
	}
	if event.StationID == "" {
		event.StationID = stationID
	}
	if event.SentAt == 0 {
		event.SentAt = time.Now().UnixMilli()
	}

	m.broker.mu.RLock()
	subs := append([]subscriberInfo{}, m.broker.subscribers[stationID]...)
	m.broker.mu.RUnlock()

	for _, sub := range subs {
		s := sub
		if event.SenderID != s.instanceID {
			go s.callback(event)
		}
	}
	return nil
}

type memCloser struct {
	broker    *MemoryBroker
	stationID string
	idx       int
}

func (mc *memCloser) Close() error {
	mc.broker.mu.Lock()
	defer mc.broker.mu.Unlock()
	subs := mc.broker.subscribers[mc.stationID]
	if mc.idx < len(subs) {
		mc.broker.subscribers[mc.stationID] = append(subs[:mc.idx], subs[mc.idx+1:]...)
	}
	return nil
}

func (m *MemorySyncStorage) SubscribeEvents(ctx context.Context, stationID string, onMessage func(event *RedisEvent)) (io.Closer, error) {
	m.broker.mu.Lock()
	defer m.broker.mu.Unlock()
	idx := len(m.broker.subscribers[stationID])
	m.broker.subscribers[stationID] = append(m.broker.subscribers[stationID], subscriberInfo{
		instanceID: m.instanceID,
		callback:   onMessage,
	})
	return &memCloser{broker: m.broker, stationID: stationID, idx: idx}, nil
}

func (m *MemorySyncStorage) SavePlaybackState(ctx context.Context, stationID string, state *model.PlaybackState) error {
	m.broker.mu.Lock()
	defer m.broker.mu.Unlock()
	copyState := *state
	m.broker.states[stationID] = &copyState
	return nil
}

func (m *MemorySyncStorage) GetPlaybackState(ctx context.Context, stationID string) (*model.PlaybackState, error) {
	m.broker.mu.RLock()
	defer m.broker.mu.RUnlock()
	if s, ok := m.broker.states[stationID]; ok {
		copyState := *s
		return &copyState, nil
	}
	return nil, nil
}

func (m *MemorySyncStorage) AcquireDJLock(ctx context.Context, stationID string, clientID string, ttl time.Duration) (bool, error) {
	m.broker.mu.Lock()
	defer m.broker.mu.Unlock()
	val := fmt.Sprintf("%s@%s", clientID, m.instanceID)
	now := time.Now()
	if info, ok := m.broker.djLocks[stationID]; ok {
		if now.Before(info.expiresAt) {
			return false, nil
		}
	}
	m.broker.djLocks[stationID] = djLockInfo{
		val:       val,
		expiresAt: now.Add(ttl),
	}
	return true, nil
}

func (m *MemorySyncStorage) RefreshDJLock(ctx context.Context, stationID string, clientID string, ttl time.Duration) error {
	m.broker.mu.Lock()
	defer m.broker.mu.Unlock()
	val := fmt.Sprintf("%s@%s", clientID, m.instanceID)
	info, ok := m.broker.djLocks[stationID]
	if !ok || info.val != val || time.Now().After(info.expiresAt) {
		return fmt.Errorf("lock inexistente ou expirado")
	}
	m.broker.djLocks[stationID] = djLockInfo{
		val:       val,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (m *MemorySyncStorage) ReleaseDJLock(ctx context.Context, stationID string, clientID string) error {
	m.broker.mu.Lock()
	defer m.broker.mu.Unlock()
	val := fmt.Sprintf("%s@%s", clientID, m.instanceID)
	if info, ok := m.broker.djLocks[stationID]; ok && info.val == val {
		delete(m.broker.djLocks, stationID)
	}
	return nil
}

func (m *MemorySyncStorage) IsDJOnline(ctx context.Context, stationID string) (bool, error) {
	m.broker.mu.RLock()
	defer m.broker.mu.RUnlock()
	info, ok := m.broker.djLocks[stationID]
	if !ok {
		return false, nil
	}
	return time.Now().Before(info.expiresAt), nil
}

func (m *MemorySyncStorage) Close() error {
	m.broker.mu.Lock()
	defer m.broker.mu.Unlock()
	m.broker.subscribers = make(map[string][]subscriberInfo)
	return nil
}
