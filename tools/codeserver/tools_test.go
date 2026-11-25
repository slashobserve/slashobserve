package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (*Code, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "src")
	files := map[string]string{
		"main.go":          "package main\n\nfunc flagValue() string { return \"paymentFailure\" }\n",
		"pkg/a.go":         "package pkg\n\nvar paymentFailure = true\n",
		"pkg/b.txt":        "paymentfailure in text\n",
		"pkg/sub/c.go":     "package sub\n\n// PAYMENTFAILURE\n",
		".git/config":      "paymentFailure\n",
		"empty.txt":        "",
		"bin.dat":          "abc\x00paymentFailure\n",
		"nested/deep/x.go": "package deep\n",
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("paymentFailure outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "secret.txt"), filepath.Join(root, "link-file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(root, "link-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("main.go", filepath.Join(root, "link-inside")); err != nil {
		t.Fatal(err)
	}
	r, err := NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	return &Code{root: r}, root
}

func TestResolveRefusesEscapes(t *testing.T) {
	code, root := fixture(t)
	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	cases := []struct {
		name string
		path string
		want error
	}{
		{"parent", "..", errOutside},
		{"parent file", "../secret.txt", errOutside},
		{"deep parent", "pkg/../../secret.txt", errOutside},
		{"absolute outside", outside, errAbsolute},
		{"absolute inside", filepath.Join(root, "main.go"), errAbsolute},
		{"leading slash", "/etc/passwd", errAbsolute},
		{"symlink file out", "link-file", errOutside},
		{"symlink dir out", "link-dir/secret.txt", errOutside},
		{"nul byte", "main.go\x00.txt", errInvalid},
		{"missing", "nope.go", errNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := code.root.Resolve(tc.path); !errors.Is(err, tc.want) {
				t.Fatalf("Resolve(%q) = %v, want %v", tc.path, err, tc.want)
			}
			if _, err := code.ReadFile(tc.path, 1, 10); err == nil {
				t.Fatalf("ReadFile(%q) succeeded", tc.path)
			}
			if _, err := code.ListDir(tc.path); err == nil {
				t.Fatalf("ListDir(%q) succeeded", tc.path)
			}
			if _, err := code.Grep(context.Background(), "paymentFailure", tc.path, ""); err == nil {
				t.Fatalf("Grep(%q) succeeded", tc.path)
			}
		})
	}
}

func TestResolveAllowsPathsInsideTheRoot(t *testing.T) {
	code, _ := fixture(t)
	for _, p := range []string{"", ".", "pkg", "pkg/../main.go", "./pkg/a.go", "link-inside"} {
		if _, err := code.root.Resolve(p); err != nil {
			t.Fatalf("Resolve(%q): %v", p, err)
		}
	}
}

func TestResolveRefusesSiblingWithSharedPrefix(t *testing.T) {
	base := t.TempDir()
	for _, d := range []string{"src", "src-other"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	r, err := NewRoot(filepath.Join(base, "src"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("../src-other"); !errors.Is(err, errOutside) {
		t.Fatalf("got %v", err)
	}
}

func TestNewRootRefusesAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRoot(f); err == nil {
		t.Fatal("a file was accepted as the root")
	}
}

func TestListDir(t *testing.T) {
	code, _ := fixture(t)
	got, err := code.ListDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"main.go\n", "pkg/\n", "nested/\n", "empty.txt\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("listing lacks %q:\n%s", want, got)
		}
	}
	empty := filepath.Join(code.root.dir, "void")
	if err := os.Mkdir(empty, 0o750); err != nil {
		t.Fatal(err)
	}
	if got, _ := code.ListDir("void"); got != "(empty directory)" {
		t.Fatalf("got %q", got)
	}
	if _, err := code.ListDir("main.go"); err == nil {
		t.Fatal("listing a file succeeded")
	}
}

func TestListDirCutsLongListings(t *testing.T) {
	code, root := fixture(t)
	big := filepath.Join(root, "big")
	if err := os.Mkdir(big, 0o750); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxListEntries+20; i++ {
		if err := os.WriteFile(filepath.Join(big, fmt.Sprintf("f%04d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := code.ListDir("big")
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(got, "\n"); lines != maxListEntries+1 {
		t.Fatalf("got %d lines", lines)
	}
	if !strings.Contains(got, "[cut: 500 of 520 entries shown") {
		t.Fatalf("no cut note:\n%s", got[len(got)-200:])
	}
}

func TestReadFile(t *testing.T) {
	code, _ := fixture(t)
	got, err := code.ReadFile("main.go", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "     3\tfunc flagValue()") {
		t.Fatalf("got %q", got)
	}
	got, _ = code.ReadFile("main.go", 3, 1)
	if got != "     3\tfunc flagValue() string { return \"paymentFailure\" }\n" {
		t.Fatalf("got %q", got)
	}
	got, _ = code.ReadFile("empty.txt", 1, 10)
	if got != "(empty file)" {
		t.Fatalf("got %q", got)
	}
	got, _ = code.ReadFile("main.go", 50, 10)
	if !strings.Contains(got, "no lines at offset 50") {
		t.Fatalf("got %q", got)
	}
	if _, err := code.ReadFile("bin.dat", 1, 10); err == nil || err.Error() != "binary file" {
		t.Fatalf("binary: %v", err)
	}
	if _, err := code.ReadFile("pkg", 1, 10); err == nil || !strings.Contains(err.Error(), "list_dir") {
		t.Fatalf("dir: %v", err)
	}
}

func TestReadFileCapsLinesAndBytes(t *testing.T) {
	code, root := fixture(t)
	var lines strings.Builder
	for i := 1; i <= maxReadLines+50; i++ {
		fmt.Fprintf(&lines, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(lines.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := code.ReadFile("long.txt", 1, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, fmt.Sprintf("[cut: line limit reached; continue with offset=%d]", maxReadLines+1)) {
		t.Fatalf("no line cut note, tail %q", got[len(got)-80:])
	}
	if strings.Count(got, "\n") != maxReadLines+1 {
		t.Fatalf("got %d lines", strings.Count(got, "\n"))
	}

	wide := strings.Repeat("x", 3000) + "\n"
	if err := os.WriteFile(filepath.Join(root, "wide.txt"), []byte(strings.Repeat(wide, 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ = code.ReadFile("wide.txt", 1, 0)
	if len(got) > maxReadBytes+200 || !strings.Contains(got, "[cut: size limit reached; continue with offset=") {
		t.Fatalf("size cap: len %d, tail %q", len(got), got[len(got)-80:])
	}
}

func TestReadFileCutsAVeryLongLine(t *testing.T) {
	code, root := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "min.js"), []byte(strings.Repeat("a", 3*maxLineBytes)+"\nnext\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := code.ReadFile("min.js", 1, 0)
	first, rest, _ := strings.Cut(got, "\n")
	if !strings.HasSuffix(first, "[line cut]") || len(first) > maxLineBytes+40 {
		t.Fatalf("first line %d bytes: %q", len(first), first[len(first)-20:])
	}
	if !strings.Contains(rest, "     2\tnext") {
		t.Fatalf("line 2 lost: %q", rest)
	}
}

func TestGrep(t *testing.T) {
	code, _ := fixture(t)
	ctx := context.Background()
	got, err := code.Grep(ctx, "paymentFailure", ".", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "main.go:3: func flagValue() string { return \"paymentFailure\" }\npkg/a.go:3: var paymentFailure = true\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	got, _ = code.Grep(ctx, "(?i)paymentfailure", "pkg", "*.go")
	if got != "pkg/a.go:3: var paymentFailure = true\npkg/sub/c.go:3: // PAYMENTFAILURE\n" {
		t.Fatalf("got:\n%s", got)
	}

	got, _ = code.Grep(ctx, "paymentFailure", "pkg/a.go", "")
	if got != "pkg/a.go:3: var paymentFailure = true\n" {
		t.Fatalf("single file: %s", got)
	}

	got, _ = code.Grep(ctx, "paymentFailure", ".", "pkg/*.go")
	if got != "pkg/a.go:3: var paymentFailure = true\n" {
		t.Fatalf("path glob: %s", got)
	}

	got, _ = code.Grep(ctx, "nothing-like-this", ".", "")
	if got != "no matches" {
		t.Fatalf("got %q", got)
	}
}

func TestGrepStaysInsideTheRoot(t *testing.T) {
	code, _ := fixture(t)
	got, err := code.Grep(context.Background(), "outside", ".", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "no matches" {
		t.Fatalf("a symlink out of the root was followed: %q", got)
	}
}

func TestGrepSkipsGitAndBinaryFiles(t *testing.T) {
	code, _ := fixture(t)
	got, _ := code.Grep(context.Background(), "paymentFailure", ".", "")
	for _, bad := range []string{".git", "bin.dat"} {
		if strings.Contains(got, bad) {
			t.Fatalf("searched %s:\n%s", bad, got)
		}
	}
}

func TestGrepRejectsBadInput(t *testing.T) {
	code, _ := fixture(t)
	if _, err := code.Grep(context.Background(), "(", ".", ""); err == nil {
		t.Fatal("bad pattern accepted")
	}
	if _, err := code.Grep(context.Background(), "x", ".", "[a"); err == nil {
		t.Fatal("bad glob accepted")
	}
}

func TestGrepCapsMatches(t *testing.T) {
	code, root := fixture(t)
	body := strings.Repeat("needle here\n", maxGrepMatches+30)
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := code.Grep(context.Background(), "needle", ".", "")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, "many.txt:"); n != maxGrepMatches {
		t.Fatalf("got %d matches", n)
	}
	if !strings.Contains(got, "[cut: stopped after 100 matches") {
		t.Fatalf("no cut note: %q", got[len(got)-120:])
	}
}

func TestGrepCapsOutputBytes(t *testing.T) {
	code, root := fixture(t)
	line := strings.Repeat("y", 1000) + " needle\n"
	if err := os.WriteFile(filepath.Join(root, "wide.txt"), []byte(strings.Repeat(line, 80)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := code.Grep(context.Background(), "needle", "wide.txt", "")
	if len(got) > maxGrepBytes+200 || !strings.Contains(got, "[cut:") {
		t.Fatalf("len %d, tail %q", len(got), got[len(got)-100:])
	}
	if !strings.Contains(got, "...") {
		t.Fatal("long match text was not clipped")
	}
}

func TestGrepSaysWhenLargeFilesWereSkipped(t *testing.T) {
	code, root := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "huge.log"), []byte(strings.Repeat("a\n", maxGrepFile)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := code.Grep(context.Background(), "paymentFailure", ".", "huge.log")
	if !strings.Contains(got, "1 files over") {
		t.Fatalf("got %q", got)
	}
}

func TestGrepStopsWhenTheCallIsCancelled(t *testing.T) {
	code, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := code.Grep(ctx, "paymentFailure", ".", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
