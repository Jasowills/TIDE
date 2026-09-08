package state

import (
	"context"

	"github.com/tide-telematics/tide/internal/resilience"
)

// Fallback serves hot state from primary (Redis) until sustained errors trip
// the breaker, then from memory until a probe succeeds (ADV-0003). Hot state
// is disposable by design (§2.6): the memory copy rebuilds from live points,
// and per-point updates self-heal any staleness after flip-back.
type Fallback struct {
	primary  Store
	fallback *MemoryStore
	cb       *resilience.Circuit
}

func NewFallback(primary Store, fallback *MemoryStore) *Fallback {
	return &Fallback{primary: primary, fallback: fallback, cb: resilience.NewCircuit("state")}
}

func (f *Fallback) Get(ctx context.Context, vehicleID string) (VehicleState, bool, error) {
	if !f.cb.InFallback() {
		st, ok, err := f.primary.Get(ctx, vehicleID)
		if err == nil {
			f.cb.NoteSuccess()
			return st, ok, nil
		}
		f.cb.NoteFailure()
		return f.fallback.Get(ctx, vehicleID)
	}
	if f.cb.ProbeDue() {
		if st, ok, err := f.primary.Get(ctx, vehicleID); err == nil {
			f.cb.NoteSuccess()
			return st, ok, nil
		}
		f.cb.Extend()
	}
	return f.fallback.Get(ctx, vehicleID)
}

func (f *Fallback) Set(ctx context.Context, s VehicleState) error {
	if !f.cb.InFallback() {
		if err := f.primary.Set(ctx, s); err == nil {
			f.cb.NoteSuccess()
			return nil
		}
		f.cb.NoteFailure()
		return f.fallback.Set(ctx, s)
	}
	if f.cb.ProbeDue() {
		if err := f.primary.Set(ctx, s); err == nil {
			f.cb.NoteSuccess()
			return nil
		}
		f.cb.Extend()
	}
	return f.fallback.Set(ctx, s)
}

// ListIDs satisfies the offline-sweeper contract against whichever backend
// is currently active.
func (f *Fallback) ListIDs(ctx context.Context) ([]string, error) {
	if !f.cb.InFallback() {
		if l, ok := f.primary.(interface {
			ListIDs(context.Context) ([]string, error)
		}); ok {
			ids, err := l.ListIDs(ctx)
			if err == nil {
				f.cb.NoteSuccess()
				return ids, nil
			}
			f.cb.NoteFailure()
			return f.fallback.ListIDs(ctx)
		}
	}
	return f.fallback.ListIDs(ctx)
}
