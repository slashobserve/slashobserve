package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func within(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s did not return", name)
	}
}

func TestAFifoIsRefusedAndNeverBlocks(t *testing.T) {
	code, root := fixture(t)
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Skip("no fifo support:", err)
	}
	ctx := context.Background()
	within(t, "read_file", func() {
		if _, err := code.ReadFile(ctx, "pipe", 1, 10); !errors.Is(err, errNotFile) {
			t.Errorf("read_file: %v", err)
		}
	})
	within(t, "list_dir", func() {
		if _, err := code.ListDir("pipe"); !errors.Is(err, errNotDir) {
			t.Errorf("list_dir: %v", err)
		}
	})
	within(t, "grep", func() {
		got, err := code.Grep(ctx, "paymentFailure", ".", "")
		if err != nil || !strings.Contains(got, "main.go:3:") {
			t.Errorf("grep over a tree with a fifo: %q, %v", got, err)
		}
	})
	within(t, "grep on the fifo", func() {
		if got, _ := code.Grep(ctx, "x", "pipe", ""); got != "no matches" {
			t.Errorf("got %q", got)
		}
	})
}

func TestReadFileStopsWhenCancelled(t *testing.T) {
	code, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := code.ReadFile(ctx, "main.go", 1, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestReadFileRefusesAFileOverTheCap(t *testing.T) {
	code, root := fixture(t)
	big := filepath.Join(root, "big.bin")
	if err := os.WriteFile(big, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, maxReadFile+1); err != nil {
		t.Fatal(err)
	}
	_, err := code.ReadFile(context.Background(), "big.bin", 1, 10)
	if err == nil || !strings.Contains(err.Error(), "grep") {
		t.Fatalf("got %v", err)
	}
}

func TestDoubleStarCrossesDirectories(t *testing.T) {
	code, _ := fixture(t)
	ctx := context.Background()
	cases := []struct {
		glob string
		want []string
	}{
		{"**/*.go", []string{"main.go:3:", "pkg/a.go:3:", "pkg/sub/c.go:3:"}},
		{"pkg/**/*.go", []string{"pkg/a.go:3:", "pkg/sub/c.go:3:"}},
		{"pkg/**", []string{"pkg/a.go:3:", "pkg/b.txt:1:", "pkg/sub/c.go:3:"}},
		{"pkg/*.go", []string{"pkg/a.go:3:"}},
		{"*.go", []string{"main.go:3:", "pkg/a.go:3:", "pkg/sub/c.go:3:"}},
	}
	for _, tc := range cases {
		got, err := code.Grep(ctx, "(?i)paymentFailure", ".", tc.glob)
		if err != nil {
			t.Fatalf("%s: %v", tc.glob, err)
		}
		var files []string
		for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
			location, _, _ := strings.Cut(line, " ")
			files = append(files, location)
		}
		if strings.Join(files, ",") != strings.Join(tc.want, ",") {
			t.Errorf("glob %q matched %v, want %v", tc.glob, files, tc.want)
		}
	}
}

func TestDoubleStarInsideASegmentIsRefused(t *testing.T) {
	code, _ := fixture(t)
	for _, g := range []string{"**.go", "pkg/a**", "a**/b"} {
		_, err := code.Grep(context.Background(), "x", ".", g)
		if err == nil || !strings.Contains(err.Error(), "whole path segment") {
			t.Errorf("glob %q: %v", g, err)
		}
	}
}

func TestGrepMatchesTheWholeLongLineButNotTheCutMarker(t *testing.T) {
	code, root := fixture(t)
	line := strings.Repeat("a", maxLineBytes+500) + " needle\n"
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, _ := code.Grep(ctx, "needle$", "long.txt", "")
	if !strings.HasPrefix(got, "long.txt:1: ") {
		t.Fatalf("a match past the read_file line cap was missed: %q", got)
	}
	if got, _ := code.Grep(ctx, `line cut`, "long.txt", ""); got != "no matches" {
		t.Fatalf("matched the cut marker: %q", got)
	}
	shown, _ := code.ReadFile(ctx, "long.txt", 1, 1)
	if !strings.Contains(shown, "[line cut]") {
		t.Fatalf("read_file does not mark the cut line: %q", shown[len(shown)-40:])
	}
}

func TestCallerMistakesAreRefusalsNotServerErrors(t *testing.T) {
	code, root := fixture(t)
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cases := []struct {
		name string
		path string
		want error
	}{
		{"file used as a directory", "main.go/x", errNotDir},
		{"symlink loop", "loop", errInvalid},
		{"name too long", strings.Repeat("a", 5000), errInvalid},
	}
	for _, tc := range cases {
		if _, err := code.ReadFile(ctx, tc.path, 1, 10); !errors.Is(err, tc.want) {
			t.Errorf("%s: read_file: %v, want %v", tc.name, err, tc.want)
		}
		if _, err := code.ListDir(tc.path); !errors.Is(err, tc.want) {
			t.Errorf("%s: list_dir: %v, want %v", tc.name, err, tc.want)
		}
		if _, err := code.Grep(ctx, "x", tc.path, ""); !errors.Is(err, tc.want) {
			t.Errorf("%s: grep: %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := code.ListDir("main.go"); !errors.Is(err, errNotDir) {
		t.Errorf("list_dir on a file: %v", err)
	}
}

func TestSymlinksInsideTheRoot(t *testing.T) {
	code, root := fixture(t)
	links := map[string]string{
		"alias":      "pkg",
		"pkg/up":     "..",
		"first-link": "main.go",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()

	listing, err := code.ListDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"alias/\n", "first-link\n", "link-dir\n", "link-file\n"} {
		if !strings.Contains(listing, want) {
			t.Errorf("listing lacks %q:\n%s", want, listing)
		}
	}

	var got string
	within(t, "grep with a symlink loop", func() {
		got, err = code.Grep(ctx, "paymentFailure", ".", "")
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"first-link:3:", "main.go:3:"} {
		if !strings.Contains(got, want) {
			t.Errorf("grep lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "alias/a.go:3:") == strings.Contains(got, "pkg/a.go:3:") {
		t.Errorf("grep must search a directory reached by two paths once:\n%s", got)
	}
	if strings.Contains(got, "up/") || strings.Contains(got, "outside") {
		t.Errorf("grep followed a loop or left the root:\n%s", got)
	}
	if strings.Contains(got, "link-file") || strings.Contains(got, "link-dir") {
		t.Errorf("grep followed a link out of the root:\n%s", got)
	}
}

func TestADeepSymlinkChainStops(t *testing.T) {
	code, root := fixture(t)
	for i := 0; i < 3; i++ {
		if err := os.Mkdir(filepath.Join(root, "d"+string(rune('a'+i))), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range [][2]string{{"da/to-b", "../db"}, {"db/to-c", "../dc"}, {"dc/to-a", "../da"}} {
		if err := os.Symlink(l[1], filepath.Join(root, l[0])); err != nil {
			t.Fatal(err)
		}
	}
	within(t, "grep over a link cycle", func() {
		if _, err := code.Grep(context.Background(), "x", ".", ""); err != nil {
			t.Error(err)
		}
	})
}

func TestAFanOutOfLinksIsSearchedOncePerDirectory(t *testing.T) {
	code, root := fixture(t)
	const levels = 24
	for i := 0; i < levels; i++ {
		dir := filepath.Join(root, "fan", "l"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		next := "../l" + strconv.Itoa(i+1)
		for _, name := range []string{"x", "y"} {
			if err := os.Symlink(next, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	end := filepath.Join(root, "fan", "l"+strconv.Itoa(levels))
	if err := os.MkdirAll(end, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(end, "deep.go"), []byte("package deep\n// fanout-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got string
	var err error
	within(t, "grep over a fan-out of links", func() {
		got, err = code.Grep(context.Background(), "fanout-marker", "fan", "")
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, "deep.go:2:"); n != 1 {
		t.Errorf("deep.go matched %d times, want 1:\n%s", n, got)
	}
}
