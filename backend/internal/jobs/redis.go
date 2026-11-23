package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisStore implementa Store no Redis:
//
//	media:job:{id}            registro do job (JSON, 24 h)
//	media:station:{id}:jobs   jobs recentes da estação (zset por criação)
//	media:lock:{chave}        lock com prazo e dono
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore não exige o Redis no ar: o cliente reconecta sozinho.
func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisStore{client: redis.NewClient(opts)}, nil
}

func (s *RedisStore) Close() error { return s.client.Close() }

func jobKey(id string) string              { return "media:job:" + id }
func stationJobsKey(station string) string { return "media:station:" + station + ":jobs" }

func (s *RedisStore) Save(ctx context.Context, rec Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	list := stationJobsKey(rec.StationID)
	_, err = s.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, jobKey(rec.ID), data, recordTTL)
		p.ZAdd(ctx, list, redis.Z{Score: float64(rec.CreatedAt), Member: rec.ID})
		p.ZRemRangeByRank(ctx, list, 0, -maxPerStation-1)
		p.Expire(ctx, list, recordTTL)
		return nil
	})
	return err
}

func (s *RedisStore) Delete(ctx context.Context, rec Record) error {
	_, err := s.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, jobKey(rec.ID))
		p.ZRem(ctx, stationJobsKey(rec.StationID), rec.ID)
		return nil
	})
	return err
}

func (s *RedisStore) Get(ctx context.Context, jobID string) (Record, bool, error) {
	data, err := s.client.Get(ctx, jobKey(jobID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Record{}, false, err
	}
	return rec, true, nil
}

func (s *RedisStore) ListByStation(ctx context.Context, stationID string) ([]Record, error) {
	ids, err := s.client.ZRevRange(ctx, stationJobsKey(stationID), 0, maxPerStation-1).Result()
	if err != nil || len(ids) == 0 {
		return []Record{}, err
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = jobKey(id)
	}
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	list := make([]Record, 0, len(values))
	for _, v := range values {
		text, ok := v.(string)
		if !ok {
			continue // o registro expirou
		}
		var rec Record
		if json.Unmarshal([]byte(text), &rec) == nil {
			list = append(list, rec)
		}
	}
	return list, nil
}

func (s *RedisStore) Lock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	l := &redisLock{client: s.client, key: "media:lock:" + key, owner: uuid.New().String()}
	ok, err := s.client.SetNX(ctx, l.key, l.owner, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	return l, true, nil
}

type redisLock struct {
	client *redis.Client
	key    string
	owner  string
}

var (
	refreshScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		end
		return 0`)
	releaseScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		end
		return 0`)
)

func (l *redisLock) Refresh(ctx context.Context, ttl time.Duration) (bool, error) {
	n, err := refreshScript.Run(ctx, l.client, []string{l.key}, l.owner, ttl.Milliseconds()).Int()
	return n == 1, err
}

func (l *redisLock) Release(ctx context.Context) error {
	return releaseScript.Run(ctx, l.client, []string{l.key}, l.owner).Err()
}
