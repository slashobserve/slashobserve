package clickhouse_test

import (
	"context"
	"errors"
	"net"
	"regexp"
	"sync"
	"sync/atomic"
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

func TestLintReportsAnUnreachableServerAsAnError(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	ds, err := clickhouse.New(clickhouse.Options{Addr: addr, Username: "u", Password: datasource.NewSecret("p")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	diags, err := ds.Lint(context.Background(), "SELECT 1")
	if !errors.Is(err, datasource.ErrUnavailable) || diags != nil {
		t.Fatalf("Lint = %+v, %v; want ErrUnavailable and no diagnostics", diags, err)
	}
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

func TestDefaultPoolServesTheFullUpstreamConcurrencyAtOnce(t *testing.T) {
	t.Parallel()
	const concurrentQueries = 32
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
	ds, err := clickhouse.New(clickhouse.Options{Addr: l.Addr().String(), Username: "u", Password: datasource.NewSecret("p")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for range concurrentQueries {
		wg.Go(func() { _, _ = ds.QueryInstant(ctx, query("SELECT 1"), lastHour()) })
	}
	wg.Wait()
	if got := accepted.Load(); got < concurrentQueries {
		t.Fatalf("connections opened = %d, want at least %d so no query waits for a connection", got, concurrentQueries)
	}
}
