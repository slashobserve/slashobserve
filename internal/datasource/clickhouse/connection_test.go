package clickhouse_test

import (
	"context"
	"errors"
	"net"
	"regexp"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
)

const driverDialTimeout = 5 * time.Second

func silentServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				<-done
				_ = c.Close()
			}()
		}
	}()
	t.Cleanup(func() {
		close(done)
		_ = l.Close()
	})
	return l.Addr().String()
}

func TestCallerDeadlineExpiringDuringConnectionSetupIsATimeout(t *testing.T) {
	t.Parallel()
	ds, err := clickhouse.New(clickhouse.Options{Addr: silentServer(t), Username: "u", Password: datasource.NewSecret("p")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	_, err = ds.QueryInstant(ctx, query("SELECT 1"), lastHour())
	elapsed := time.Since(start)
	if !errors.Is(err, datasource.ErrTimeout) || !regexp.MustCompile(`after \d+(\.\d+)?(ms|s)`).MatchString(err.Error()) {
		t.Fatalf("err = %v, want ErrTimeout naming the deadline", err)
	}
	if elapsed >= 2*driverDialTimeout {
		t.Fatalf("returned after %s, want under %s", elapsed, 2*driverDialTimeout)
	}
}
