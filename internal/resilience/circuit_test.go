package resilience

import (
	"testing"
	"time"
)

func TestCircuitTripsAndRecovers(t *testing.T) {
	c := &Circuit{Threshold: 3, Cooldown: 50 * time.Millisecond, Name: "test"}
	if c.InFallback() {
		t.Fatal("fresh circuit must use primary")
	}
	c.NoteFailure()
	c.NoteFailure()
	if c.InFallback() {
		t.Fatal("below threshold must not trip")
	}
	if !c.NoteFailure() {
		t.Fatal("threshold trip should report newly engaged")
	}
	if !c.InFallback() {
		t.Fatal("must be in fallback after threshold")
	}
	if c.ProbeDue() {
		t.Fatal("probe not due inside cooldown")
	}
	time.Sleep(60 * time.Millisecond)
	if !c.ProbeDue() {
		t.Fatal("probe due after cooldown")
	}
	c.NoteSuccess()
	if c.InFallback() || c.ProbeDue() {
		t.Fatal("success must reset to primary")
	}
}
