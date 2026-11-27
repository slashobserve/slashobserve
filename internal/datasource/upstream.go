package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const maxUpstreamTextBytes = 512

type timeoutKey struct{}

func Bound(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	reported := timeout
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
		reported = 0
	}
	ctx = context.WithValue(ctx, timeoutKey{}, reported)
	return context.WithTimeout(ctx, timeout)
}

func TransportError(ctx context.Context, err error) error {
	reported, _ := ctx.Value(timeoutKey{}).(time.Duration)
	if cerr := ContextError(ctx, err, reported); cerr != nil {
		return cerr
	}
	return Errorf(ErrUnavailable, "connection failed")
}

func TruncateText(s string) string {
	if len(s) > maxUpstreamTextBytes {
		s = s[:maxUpstreamTextBytes]
	}
	return strings.ToValidUTF8(s, "")
}

func UpstreamText(raw []byte) string {
	var body struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	text := string(bytes.TrimSpace(raw))
	if json.Unmarshal(raw, &body) == nil {
		text = body.Message
		if text == "" {
			text = body.Error
		}
	}
	return TruncateText(text)
}

func Unreadable() error {
	return Errorf(ErrUnavailable, "upstream returned an unreadable result")
}

func MessageOf(err error) string {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Message
	}
	return err.Error()
}

func FilterSortLimit(values []string, search string, limit int) (kept []string, cut bool) {
	search = strings.ToLower(search)
	out := make([]string, 0, len(values))
	for _, v := range values {
		if search == "" || strings.Contains(strings.ToLower(v), search) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > limit {
		return out[:limit], true
	}
	return out, false
}

func Clamp(v, upper int) int {
	return max(0, min(v, upper))
}

func InstantInterval(r TimeRange, minimum time.Duration) time.Duration {
	return max(time.Second, (r.To.Sub(r.From) / 1000).Truncate(time.Second), minimum)
}
