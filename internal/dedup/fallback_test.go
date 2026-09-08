package dedup

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubStore struct {
	fail bool
	seen map[string]bool
}

func (s *stubStore) Seen(_ context.Context, key string) (bool, error) {
	if s.fail {
		return false, errors.New("redis down")
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	dup := s.seen[key]
	s.seen[key] = true
	return dup, nil
}

// ADV-0003: sustained primary errors fail over to memory; recovery flips back.
func TestFallbackFailoverAndRecovery(t *testing.T) {
	ctx := context.Background()
	primary := &stubStore{}
	fb := NewFallback(primary, NewMemoryStore(0))
	fb.cb.Threshold = 2
	fb.cb.Cooldown = 30 * time.Millisecond

	if dup, err := fb.Seen(ctx, "a"); err != nil || dup {
		t.Fatalf("primary path: dup=%v err=%v", dup, err)
	}
	primary.fail = true
	// Two failures trip the breaker; calls keep serving from memory.
	for i := 0; i < 3; i++ {
		if _, err := fb.Seen(ctx, "b"); err != nil {
			t.Fatalf("fallback must serve during outage: %v", err)
		}
	}
	if !fb.cb.InFallback() {
		t.Fatal("breaker should be open")
	}
	primary.fail = false
	time.Sleep(40 * time.Millisecond) // cooldown elapses
	if dup, err := fb.Seen(ctx, "c"); err != nil || dup {
		t.Fatalf("recovered path: dup=%v err=%v", dup, err)
	}
	if fb.cb.InFallback() {
		t.Fatal("breaker should have closed on probe success")
	}
}
