package logvolume_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/logvolume"
)

var webLevelWords = map[logvolume.Kind][]string{
	logvolume.Error:   {"err", "eror", "error", "emerg", "fatal", "alert", "crit", "critical", "panic"},
	logvolume.Warning: {"warn", "warning"},
	logvolume.Info:    {"info", "information", "informational", "notice"},
}

func TestKindOfMapsEveryWebLevelWord(t *testing.T) {
	for kind, words := range webLevelWords {
		for _, w := range words {
			for _, variant := range []string{w, "  " + w + "\t", strings.ToUpper(w)} {
				if got := logvolume.KindOf(variant); got != kind {
					t.Errorf("KindOf(%q) = %q, want %q", variant, got, kind)
				}
			}
		}
	}
}

func TestKindOfUnknownWordsAreOther(t *testing.T) {
	for _, w := range []string{"Debug", "TRACE", "", "custom", "dbug"} {
		if got := logvolume.KindOf(w); got != logvolume.Other {
			t.Errorf("KindOf(%q) = %q, want other", w, got)
		}
	}
}

func TestAliasesMatchTheWebListAndAreCopies(t *testing.T) {
	for _, kind := range logvolume.Kinds {
		if got := logvolume.Aliases(kind); !slices.Equal(got, webLevelWords[kind]) {
			t.Errorf("Aliases(%q) = %v, want %v", kind, got, webLevelWords[kind])
		}
	}
	logvolume.Aliases(logvolume.Error)[0] = "changed"
	if logvolume.Aliases(logvolume.Error)[0] != "err" {
		t.Fatal("Aliases returned the shared slice")
	}
}

func TestFreshVolumeHasEmptyBucketsSteppedFromFrom(t *testing.T) {
	from := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	v := logvolume.New(from, time.Minute, 3).Volume()
	if v.StepMS != 60000 || v.Total != 0 || len(v.Buckets) != 3 {
		t.Fatalf("volume = %+v", v)
	}
	for i, b := range v.Buckets {
		if want := from.Add(time.Duration(i) * time.Minute); !b.Start.Equal(want) || b != (logvolume.Bucket{Start: want}) {
			t.Errorf("bucket %d = %+v, want empty at %v", i, b, want)
		}
	}
	if empty := logvolume.New(from, time.Minute, 0).Volume(); empty.Buckets == nil {
		t.Fatal("zero buckets gave a nil slice")
	}
}

func TestAddFilesCountsByBucketAndKind(t *testing.T) {
	from := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	h := logvolume.New(from, time.Minute, 3)
	h.Add(from, logvolume.Error, 2)
	h.Add(from.Add(59*time.Second), logvolume.Warning, 1)
	h.Add(from.Add(2*time.Minute+30*time.Second), logvolume.Info, 4)
	h.Add(from.Add(2*time.Minute), logvolume.Other, 5)
	h.Add(from.Add(-time.Nanosecond), logvolume.Error, 100)
	h.Add(from.Add(3*time.Minute), logvolume.Error, 100)
	v := h.Volume()
	want := []logvolume.Bucket{
		{Start: from, Error: 2, Warning: 1},
		{Start: from.Add(time.Minute)},
		{Start: from.Add(2 * time.Minute), Info: 4, Other: 5},
	}
	if !slices.Equal(v.Buckets, want) {
		t.Fatalf("buckets = %+v, want %+v", v.Buckets, want)
	}
	if v.Total != 12 {
		t.Fatalf("total = %d, want 12", v.Total)
	}
}
