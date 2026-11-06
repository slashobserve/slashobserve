package loki_test

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

func TestVolumeQueryWrapsALogQueryInACountByLevel(t *testing.T) {
	got, err := loki.VolumeQuery(`{service_name="checkout"} |= "error"`, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	want := "sum by (level, lvl, severity_text, severity, detected_level) (count_over_time(\n" +
		`{service_name="checkout"} |= "error"` + "\n[60000ms]))"
	if got != want {
		t.Fatalf("query =\n%s\nwant\n%s", got, want)
	}
}

func TestVolumeQueryClosesAfterATrailingComment(t *testing.T) {
	got, err := loki.VolumeQuery("{app=\"shop\"} | json # note", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := "sum by (level, lvl, severity_text, severity, detected_level) (count_over_time(\n{app=\"shop\"} | json # note\n[5000ms]))"
	if got != want {
		t.Fatalf("query =\n%s\nwant\n%s", got, want)
	}
}

func TestVolumeQueryRefusesMetricQueries(t *testing.T) {
	for _, q := range []string{`rate({a="b"}[1m])`, "# total\nsum(count_over_time({a=\"b\"}[1m]))"} {
		_, err := loki.VolumeQuery(q, time.Minute)
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("VolumeQuery(%q) err = %v, want invalid query", q, err)
		}
		if want := `invalid query: the volume needs a log query that starts with a stream selector, such as {service_name="checkout"}`; err.Error() != want {
			t.Fatalf("err = %q, want %q", err, want)
		}
	}
}

func TestVolumeLevelPicksTheFirstLevelLabel(t *testing.T) {
	if got := loki.VolumeLevel(map[string]string{"detected_level": "info", "level": "error"}); got != "error" {
		t.Fatalf("level = %q, want error", got)
	}
	if got := loki.VolumeLevel(map[string]string{"detected_level": "warn"}); got != "warn" {
		t.Fatalf("level = %q, want warn", got)
	}
	if got := loki.VolumeLevel(map[string]string{"service_name": "checkout"}); got != "" {
		t.Fatalf("level = %q, want empty", got)
	}
}
