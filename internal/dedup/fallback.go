package dedup

import (
	"context"

	"github.com/tide-telematics/tide/internal/resilience"
)

// Fallback serves dedup from primary (Redis) until sustained errors trip the
// breaker, then from memory until a probe succeeds (ADV-0003). A lost dedup
// window only risks redelivery, which deterministic event IDs + PG upserts
// absorb per the at-least-once contract.
type Fallback struct {
	primary  Store
	fallback *MemoryStore
	cb       *resilience.Circuit
}

func NewFallback(primary Store, fallback *MemoryStore) *Fallback {
	return &Fallback{primary: primary, fallback: fallback, cb: resilience.NewCircuit("dedup")}
}

func (f *Fallback) Seen(ctx context.Context, key string) (bool, error) {
	if !f.cb.InFallback() {
		dup, err := f.primary.Seen(ctx, key)
		if err == nil {
			f.cb.NoteSuccess()
			return dup, nil
		}
		f.cb.NoteFailure()
		return f.fallback.Seen(ctx, key)
	}
	if f.cb.ProbeDue() {
		if dup, err := f.primary.Seen(ctx, key); err == nil {
			f.cb.NoteSuccess()
			return dup, nil
		}
		f.cb.Extend()
	}
	return f.fallback.Seen(ctx, key)
}
