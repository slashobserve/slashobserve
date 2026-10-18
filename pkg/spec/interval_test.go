package spec_test

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestParseInterval(t *testing.T) {
	valid := map[string]time.Duration{
		"15s":    15 * time.Second,
		"100ms":  100 * time.Millisecond,
		"5m":     5 * time.Minute,
		"1h":     time.Hour,
		"1d":     24 * time.Hour,
		"2w":     14 * 24 * time.Hour,
		"1y":     365 * 24 * time.Hour,
		"99999d": 99999 * 24 * time.Hour,
		"252y":   252 * 365 * 24 * time.Hour,
	}
	for in, want := range valid {
		got, err := spec.ParseInterval(in)
		if err != nil || got != want {
			t.Errorf("ParseInterval(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0s", "1M", "1.5s", "123456s", "5", "s", "-5s", " 5s", "300y", "99999w"} {
		if got, err := spec.ParseInterval(in); err == nil {
			t.Errorf("ParseInterval(%q) = %v, want an error", in, got)
		}
	}
}
