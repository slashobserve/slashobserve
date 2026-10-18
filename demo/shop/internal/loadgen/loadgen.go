package loadgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultRPS     = 20
	maxRPS         = 1000
	defaultSeed    = 42
	maxInFlight    = 200
	requestTimeout = 7 * time.Second
	drainTimeout   = 8 * time.Second
	summaryEvery   = 30 * time.Second
	startPoll      = 200 * time.Millisecond
	startTimeout   = 5 * time.Minute
	maxStartLead   = time.Minute
	emptyCartPct   = 3
	productCount   = 20
	orderCount     = 50
)

type Config struct {
	Target    string
	RPS       int
	Seed      uint64
	StartFile string
}

func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	get := func(key string) string {
		value, _ := lookup(key)
		return strings.TrimSpace(value)
	}
	cfg := Config{
		Target:    strings.TrimRight(get("LOADGEN_TARGET"), "/"),
		RPS:       defaultRPS,
		Seed:      defaultSeed,
		StartFile: get("LOADGEN_START_FILE"),
	}
	if cfg.Target == "" {
		return Config{}, errors.New("LOADGEN_TARGET is required")
	}
	if raw := get("LOADGEN_RPS"); raw != "" {
		rps, err := strconv.Atoi(raw)
		if err != nil || rps < 1 || rps > maxRPS {
			return Config{}, fmt.Errorf("LOADGEN_RPS must be an integer between 1 and %d, got %q", maxRPS, raw)
		}
		cfg.RPS = rps
	}
	if raw := get("LOADGEN_SEED"); raw != "" {
		seed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("LOADGEN_SEED must be an unsigned integer, got %q", raw)
		}
		cfg.Seed = seed
	}
	return cfg, nil
}

type plan struct {
	method string
	path   string
	body   []byte
}

type counters struct {
	sent, ok, clientErr, serverErr, failed, dropped atomic.Int64
}

func Run(ctx context.Context, lookup func(string) (string, bool), logger *slog.Logger) error {
	cfg, err := ConfigFromEnv(lookup)
	if err != nil {
		return err
	}
	logger.Info("loadgen started", "target", cfg.Target, "rps", cfg.RPS, "seed", cfg.Seed)
	if cfg.StartFile != "" {
		if err := waitForStart(ctx, cfg.StartFile, logger); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = maxInFlight
	client := &http.Client{Transport: transport}

	var (
		stats    counters
		inflight sync.WaitGroup
		slots    = make(chan struct{}, maxInFlight)
		rng      = rand.New(rand.NewPCG(cfg.Seed, cfg.Seed)) //nolint:gosec // deterministic seeded demo traffic, not security sensitive
		tick     = time.NewTicker(time.Second / time.Duration(cfg.RPS))
		summary  = time.NewTicker(summaryEvery)
	)
	defer tick.Stop()
	defer summary.Stop()

	for {
		select {
		case <-ctx.Done():
			waitInFlight(&inflight)
			logSummary(logger, "loadgen stopped", &stats)
			return nil
		case <-summary.C:
			logSummary(logger, "loadgen summary", &stats)
		case <-tick.C:
			p := nextPlan(rng)
			select {
			case slots <- struct{}{}:
				inflight.Go(func() {
					defer func() { <-slots }()
					send(client, cfg.Target, p, &stats)
				})
			default:
				stats.dropped.Add(1)
			}
		}
	}
}

func waitForStart(ctx context.Context, path string, logger *slog.Logger) error {
	deadline := time.Now().Add(startTimeout)
	for {
		raw, err := os.ReadFile(path) //nolint:gosec // path comes from the operator's own environment
		if err == nil {
			seconds, parseErr := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
			if parseErr != nil {
				return fmt.Errorf("LOADGEN_START_FILE %s: %w", path, parseErr)
			}
			return sleepUntil(ctx, time.Unix(seconds, 0), logger)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read LOADGEN_START_FILE: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("LOADGEN_START_FILE %s did not appear within %s", path, startTimeout)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(startPoll):
		}
	}
}

func sleepUntil(ctx context.Context, start time.Time, logger *slog.Logger) error {
	wait := time.Until(start)
	if wait <= 0 {
		return nil
	}
	if wait > maxStartLead {
		logger.Warn("loadgen start time is too far ahead; starting now", "start", start.UTC().Format(time.RFC3339), "wait", wait.String(), "max", maxStartLead.String())
		return nil
	}
	logger.Info("loadgen waiting for the backfill to end", "start", start.UTC().Format(time.RFC3339), "wait", wait.String())
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
	return nil
}

func waitInFlight(inflight *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(drainTimeout):
	}
}

func logSummary(logger *slog.Logger, message string, stats *counters) {
	logger.Info(message,
		"sent", stats.sent.Load(),
		"ok", stats.ok.Load(),
		"client_errors", stats.clientErr.Load(),
		"server_errors", stats.serverErr.Load(),
		"failed", stats.failed.Load(),
		"dropped", stats.dropped.Load(),
	)
}

func nextPlan(rng *rand.Rand) plan {
	switch roll := rng.IntN(100); {
	case roll < 50:
		return plan{method: http.MethodGet, path: "/api/products"}
	case roll < 75:
		return plan{method: http.MethodGet, path: "/api/products/" + strconv.Itoa(1+rng.IntN(productCount))}
	case roll < 95:
		return plan{method: http.MethodPost, path: "/api/checkout", body: nextCart(rng)}
	default:
		return plan{method: http.MethodGet, path: "/api/orders/" + strconv.Itoa(1+rng.IntN(orderCount))}
	}
}

func nextCart(rng *rand.Rand) []byte {
	if rng.IntN(100) < emptyCartPct {
		return []byte(`{"items":[]}`)
	}
	items := make([]string, 1+rng.IntN(3))
	for i := range items {
		items[i] = fmt.Sprintf(`{"sku":"p%d","qty":%d}`, 1+rng.IntN(productCount), 1+rng.IntN(3))
	}
	return []byte(`{"items":[` + strings.Join(items, ",") + `]}`)
}

func send(client *http.Client, target string, p plan, stats *counters) {
	stats.sent.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, p.method, target+p.path, bytes.NewReader(p.body))
	if err != nil {
		stats.failed.Add(1)
		return
	}
	if p.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		stats.failed.Add(1)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		stats.serverErr.Add(1)
	case resp.StatusCode >= http.StatusBadRequest:
		stats.clientErr.Add(1)
	default:
		stats.ok.Add(1)
	}
}
