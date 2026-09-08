// Package resilience holds the circuit-breaker shared by rebuildable-store
// fallbacks (ADV-0003). Redis keeps nothing durable in TIDE (hot state is
// disposable per §2.6; dedup windows are absorbed downstream by deterministic
// IDs + PG upserts), so on sustained Redis errors we fail over to memory and
// probe back — ingestion stays up, recovery is automatic.
package resilience

import (
	"log"
	"sync"
	"time"
)

// Circuit trips after Threshold consecutive primary errors and routes to
// fallback for Cooldown; the first call after cooldown probes primary and
// flips back on success. Single errors never flip (transient blips stay put).
type Circuit struct {
	mu        sync.Mutex
	consec    int
	until     time.Time
	Threshold int
	Cooldown  time.Duration
	Name      string
}

func NewCircuit(name string) *Circuit {
	return &Circuit{Threshold: 3, Cooldown: 5 * time.Second, Name: name}
}

// InFallback reports whether calls should presently use fallback.
func (c *Circuit) InFallback() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.until.IsZero() && time.Now().Before(c.until)
}

// ProbeDue reports whether a primary trial is owed (a fallback window was
// set and has now elapsed).
func (c *Circuit) ProbeDue() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.until.IsZero() && !time.Now().Before(c.until)
}

// NoteSuccess resets the breaker onto primary.
func (c *Circuit) NoteSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consec = 0
	c.until = time.Time{}
}

// NoteFailure records a primary error; true when this trip newly engages
// fallback (log once, not per call).
func (c *Circuit) NoteFailure() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consec++
	if c.consec >= c.Threshold && c.until.IsZero() {
		c.until = time.Now().Add(c.Cooldown)
		log.Printf("resilience: %s circuit open, failing over (%ds cooldown)", c.Name, int(c.Cooldown.Seconds()))
		return true
	}
	return false
}

// Extend pushes the fallback window out after a failed probe.
func (c *Circuit) Extend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.until = time.Now().Add(c.Cooldown)
}
