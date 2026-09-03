// Package dedup implements Architecture §2.5 identity: sequence-first,
// hash-fallback (see schemas/telemetry DedupKey). Duplicate telemetry must
// never cause a duplicate state transition (property test in pipeline).
package dedup

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store records seen dedup keys. MemoryStore is for tests/dev; RedisStore
// backs prod (keys TTL'd — dedup window, durable log stays in Postgres).
type Store interface {
	// Seen returns true if key was already recorded (i.e. this is a duplicate).
	Seen(ctx context.Context, key string) (bool, error)
}

type MemoryStore struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func NewMemoryStore(ttl time.Duration) *MemoryStore {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &MemoryStore{seen: map[string]time.Time{}, ttl: ttl}
}

func (m *MemoryStore) Seen(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if ts, ok := m.seen[key]; ok && now.Sub(ts) < m.ttl {
		return true, nil
	}
	m.seen[key] = now
	return false, nil
}

type RedisStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisStore(addr string, ttl time.Duration) *RedisStore {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &RedisStore{rdb: redis.NewClient(&redis.Options{Addr: addr}), ttl: ttl}
}

func (r *RedisStore) Seen(ctx context.Context, key string) (bool, error) {
	ok, err := r.rdb.SetNX(ctx, "tide:dedup:"+key, "1", r.ttl).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil
}
