package loadgen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/demo/internal/shopapp"
)

const (
	markerEvery    = 6 * time.Hour
	markerLookback = 26 * time.Hour
	markerSettle   = 30 * time.Second
	markerRetry    = 15 * time.Second
	markerTimeout  = 10 * time.Second
	markerTag      = "deploy"
	markerPageSize = 100
)

var (
	markerServices = []string{shopapp.ServiceFrontend, shopapp.ServiceCheckout, shopapp.ServicePayments}
	markerChanges  = []string{
		"bump dependencies",
		"tune the connection pool",
		"retry payment timeouts",
		"cache the product list",
		"raise the queue worker count",
		"trim request logging",
		"roll out the new cart page",
		"fix an order total rounding bug",
	}
	errNotReady = errors.New("the server or its key is not ready")
)

type markerFailure struct {
	reason string
	status int
}

func (f *markerFailure) Error() string { return f.reason }

func markerTimes(now time.Time) []time.Time {
	now = now.UTC()
	earliest := now.Add(-markerLookback)
	var slots []time.Time
	for slot := now.Truncate(markerEvery); slot.After(earliest); slot = slot.Add(-markerEvery) {
		slots = append(slots, slot)
	}
	slices.Reverse(slots)
	return slots
}

func nextMarkerCheck(now time.Time) time.Time {
	return now.UTC().Truncate(markerEvery).Add(markerEvery + markerSettle)
}

func missing(slots, existing []time.Time) []time.Time {
	var out []time.Time
	for _, slot := range slots {
		if !slices.ContainsFunc(existing, slot.Equal) {
			out = append(out, slot)
		}
	}
	return out
}

func markerRand(seed uint64, slot time.Time) *rand.Rand {
	return rand.New(rand.NewPCG(seed, uint64(slot.Unix()))) //nolint:gosec // deterministic seeded demo markers, not security sensitive
}

func markerService(seed uint64, slot time.Time) string {
	return markerServices[markerRand(seed, slot).IntN(len(markerServices))]
}

func markerText(seed uint64, service string, slot time.Time) string {
	r := markerRand(seed^0x9e3779b97f4a7c15, slot)
	day := slot.Unix() / int64(24*time.Hour/time.Second)
	return fmt.Sprintf("Deploy %s v1.%d.%d: %s", service, day%100, r.IntN(10), markerChanges[r.IntN(len(markerChanges))])
}

type markerClient struct {
	base      string
	tokenFile string
	seed      uint64
	http      *http.Client
}

func runMarkers(ctx context.Context, cfg Config, logger *slog.Logger) {
	c := markerClient{base: cfg.ObserveURL, tokenFile: cfg.ObserveTokenFile, seed: cfg.Seed, http: &http.Client{Timeout: markerTimeout}}
	waiting := false
	for {
		posted, err := c.sync(ctx, time.Now())
		wait := time.Until(nextMarkerCheck(time.Now()))
		var failure *markerFailure
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, errNotReady):
			if !waiting {
				logger.Info("deploy markers waiting for the server and its key")
				waiting = true
			}
			wait = markerRetry
		case errors.As(err, &failure):
			logger.Warn("deploy markers failed", "reason", failure.reason, "status", failure.status)
		case err != nil:
			logger.Warn("deploy markers failed", "reason", "internal")
		default:
			waiting = false
			logger.Info("deploy markers checked", "posted", posted)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (c markerClient) sync(ctx context.Context, now time.Time) (int, error) {
	token, err := c.token()
	if err != nil {
		return 0, err
	}
	slots := markerTimes(now)
	existing, err := c.existing(ctx, token, slots[0])
	if err != nil {
		return 0, err
	}
	posted := 0
	for _, slot := range missing(slots, existing) {
		if err := c.post(ctx, token, slot); err != nil {
			return posted, err
		}
		posted++
	}
	return posted, nil
}

func (c markerClient) token() (string, error) {
	raw, err := os.ReadFile(c.tokenFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", errNotReady
	}
	if err != nil {
		return "", &markerFailure{reason: "token_file_unreadable"}
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", &markerFailure{reason: "token_file_empty"}
	}
	return token, nil
}

func (c markerClient) existing(ctx context.Context, token string, from time.Time) ([]time.Time, error) {
	query := url.Values{"tags": {markerTag}, "from": {from.Format(time.RFC3339)}, "limit": {fmt.Sprint(markerPageSize)}}
	var body struct {
		Annotations []struct {
			TimeFrom time.Time `json:"time_from"`
		} `json:"annotations"`
	}
	if err := c.do(ctx, token, http.MethodGet, "/api/v1/annotations?"+query.Encode(), nil, http.StatusOK, &body); err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(body.Annotations))
	for _, a := range body.Annotations {
		out = append(out, a.TimeFrom)
	}
	return out, nil
}

func (c markerClient) post(ctx context.Context, token string, slot time.Time) error {
	service := markerService(c.seed, slot)
	payload, err := json.Marshal(map[string]any{
		"time_from": slot.Format(time.RFC3339),
		"text":      markerText(c.seed, service, slot),
		"tags":      []string{markerTag, service},
	})
	if err != nil {
		return err
	}
	return c.do(ctx, token, http.MethodPost, "/api/v1/annotations", payload, http.StatusCreated, nil)
}

func (c markerClient) do(ctx context.Context, token, method, path string, payload []byte, want int, into any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errNotReady
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			return errNotReady
		}
		return &markerFailure{reason: "unexpected_status", status: resp.StatusCode}
	}
	if into == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return &markerFailure{reason: "unreadable_response", status: resp.StatusCode}
	}
	return nil
}
