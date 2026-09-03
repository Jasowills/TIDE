package observability

import (
	"context"
	"testing"
)

func TestInitAndDummySpan(t *testing.T) {
	shutdown, err := Init(context.Background(), "tide-test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	DummySpan(context.Background())
}
