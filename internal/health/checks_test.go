package health

import (
	"context"
	"testing"
)

// Unreachable targets must fail fast with OK=false — never hang the suite.
func TestChecksFailClosed(t *testing.T) {
	ctx := context.Background()
	if s := CheckPostgres(ctx, "postgres://127.0.0.1:1/tide?sslmode=disable"); s.OK {
		t.Fatal("expected postgres check to fail for bad port")
	}
	if s := CheckRedis(ctx, "127.0.0.1:1"); s.OK {
		t.Fatal("expected redis check to fail for bad port")
	}
	if s := CheckNATS("nats://127.0.0.1:14222"); s.OK {
		t.Fatal("expected nats check to fail for bad port")
	}
}
