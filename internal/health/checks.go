package health

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
	_ "github.com/jackc/pgx/v5/stdlib"
	nats "github.com/nats-io/nats.go"
)

// Check names are stable: tide doctor output + compose smoke test parse them.
const (
	NamePostgres = "postgres"
	NameRedis    = "redis"
	NameNATS     = "nats"
)

type Status struct {
	Name    string
	OK      bool
	Detail  string
}

func CheckPostgres(ctx context.Context, dsn string) Status {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return Status{NamePostgres, false, err.Error()}
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return Status{NamePostgres, false, err.Error()}
	}
	return Status{NamePostgres, true, "reachable"}
}

func CheckRedis(ctx context.Context, addr string) Status {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return Status{NameRedis, false, err.Error()}
	}
	return Status{NameRedis, true, "reachable"}
}

func CheckNATS(url string) Status {
	conn, err := nats.Connect(url, nats.Timeout(3*time.Second))
	if err != nil {
		// Distinguish "nothing listening" from other errors for clearer doctor output.
		if _, derr := net.DialTimeout("tcp", natsURLToHostPort(url), 2*time.Second); derr != nil {
			return Status{NameNATS, false, fmt.Sprintf("unreachable: %v", err)}
		}
		return Status{NameNATS, false, err.Error()}
	}
	conn.Close()
	return Status{NameNATS, true, "reachable"}
}

func natsURLToHostPort(url string) string {
	rest := url
	for _, p := range []string{"nats://", "tls://"} {
		if len(rest) > len(p) && rest[:len(p)] == p {
			rest = rest[len(p):]
			break
		}
	}
	if i := len(rest); i > 0 {
		for j, c := range rest {
			if c == '/' {
				return rest[:j]
			}
		}
	}
	return rest
}
