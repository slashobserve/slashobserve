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
	r, err := NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return &Code{root: r}, root
}

func TestEscapesAreRefused(t *testing.T) {
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
			if _, err := code.root.Stat(tc.path); !errors.Is(err, tc.want) {
				t.Fatalf("Stat(%q) = %v, want %v", tc.path, err, tc.want)
			}
			if _, err := code.ReadFile(context.Background(), tc.path, 1, 10); err == nil {
				t.Fatalf("ReadFile(%q) succeeded", tc.path)
			}
			if _, err := code.ListDir(context.Background(), tc.path); err == nil {
				t.Fatalf("ListDir(%q) succeeded", tc.path)
			}
			if _, err := code.Grep(context.Background(), "paymentFailure", tc.path, ""); err == nil {
				t.Fatalf("Grep(%q) succeeded", tc.path)
			}
		})
	}
}

func TestPathsInsideTheRootAreAllowed(t *testing.T) {
	code, root := fixture(t)
	if err := os.Symlink("main.go", filepath.Join(root, "link-inside")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", ".", "pkg", "pkg/../main.go", "./pkg/a.go", "link-inside"} {
		if _, err := code.root.Stat(p); err != nil {
			t.Fatalf("Stat(%q): %v", p, err)
		}
	}
}

func TestSiblingWithSharedPrefixIsRefused(t *testing.T) {
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
	if _, err := r.Stat("../src-other"); !errors.Is(err, errOutside) {
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
	got, err := code.ListDir(context.Background(), ".")
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
	if got, _ := code.ListDir(context.Background(), "void"); got != "(empty directory)" {
		t.Fatalf("got %q", got)
	}
	if _, err := code.ListDir(context.Background(), "main.go"); err == nil {
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
	got, err := code.ListDir(context.Background(), "big")
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
	got, err := code.ReadFile(context.Background(), "main.go", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "     3\tfunc flagValue()") {
		t.Fatalf("got %q", got)
	}
	got, _ = code.ReadFile(context.Background(), "main.go", 3, 1)
	if got != "     3\tfunc flagValue() string { return \"paymentFailure\" }\n" {
		t.Fatalf("got %q", got)
	}
	got, _ = code.ReadFile(context.Background(), "empty.txt", 1, 10)
	if got != "(empty file)" {
		t.Fatalf("got %q", got)
	}
	got, _ = code.ReadFile(context.Background(), "main.go", 50, 10)
	if !strings.Contains(got, "no lines at offset 50") {
		t.Fatalf("got %q", got)
	}
	if _, err := code.ReadFile(context.Background(), "bin.dat", 1, 10); err == nil || err.Error() != "binary file" {
		t.Fatalf("binary: %v", err)
	}
	if _, err := code.ReadFile(context.Background(), "pkg", 1, 10); err == nil || !strings.Contains(err.Error(), "list_dir") {
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
	got, err := code.ReadFile(context.Background(), "long.txt", 1, 100000)
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
	got, _ = code.ReadFile(context.Background(), "wide.txt", 1, 0)
	if len(got) > maxReadBytes+200 || !strings.Contains(got, "[cut: size limit reached; continue with offset=") {
		t.Fatalf("size cap: len %d, tail %q", len(got), got[len(got)-80:])
	}
}

func TestReadFileCutsAVeryLongLine(t *testing.T) {
	code, root := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "min.js"), []byte(strings.Repeat("a", 3*maxLineBytes)+"\nnext\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := code.ReadFile(context.Background(), "min.js", 1, 0)
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

func TestMissingPathsNameWhatExists(t *testing.T) {
	code, _ := fixture(t)
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"list under missing dir", func() error { _, err := code.ListDir(context.Background(), "src/pkg"); return err }, `not found: "src/pkg"; the root has: `},
		{"grep under missing dir", func() error { _, err := code.Grep(ctx, "x", "src/pkg", ""); return err }, `the root has: `},
		{"read typo in existing dir", func() error { _, err := code.ReadFile(ctx, "pkg/z.go", 1, 10); return err }, `not found: "pkg/z.go"; pkg has: a.go, b.txt, sub/`},
		{"read below a missing dir", func() error { _, err := code.ReadFile(ctx, "pkg/nope/x.go", 1, 10); return err }, `pkg has: a.go, b.txt, sub/`},
	}
	for _, tc := range cases {
		err := tc.call()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want it to contain %q", tc.name, err, tc.want)
		}
		var r refusal
		if !errors.As(err, &r) {
			t.Errorf("%s: %v is not a refusal the caller sees", tc.name, err)
		}
	}
}

func TestMissingPathHintIsCutAndNamesNoServerPath(t *testing.T) {
	code, root := fixture(t)
	if err := os.MkdirAll(filepath.Join(root, "many"), 0o750); err != nil {
		t.Fatal(err)
	}
	for i := range maxHintNames + 5 {
		if err := os.WriteFile(filepath.Join(root, "many", fmt.Sprintf("f%03d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := code.ListDir(context.Background(), "many/missing")
	if err == nil || !strings.Contains(err.Error(), "many has: f000") || !strings.Contains(err.Error(), ", and 5 more") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("leaks the server path: %v", err)
	}
	if _, err := code.ListDir(context.Background(), "../nope"); !errors.Is(err, errOutside) {
		t.Fatalf("an escape was explained instead of refused: %v", err)
	}
}

func TestMissingPathStillUnwrapsToNotFound(t *testing.T) {
	code, _ := fixture(t)
	if _, err := code.ListDir(context.Background(), "nope/x"); !errors.Is(err, errNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := code.ReadFile(context.Background(), "pkg/z.go", 1, 1); !errors.Is(err, errNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestMissingPathInDanglingSymlinkSaysSo(t *testing.T) {
	code, root := fixture(t)
	if err := os.Symlink("gone", filepath.Join(root, "pkg", "dangling")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"pkg/dangling", "pkg/dangling/x.go"} {
		_, err := code.ReadFile(context.Background(), rel, 1, 1)
		if err == nil || !strings.Contains(err.Error(), `"dangling" in pkg is a symlink whose target does not exist`) {
			t.Errorf("%s: got %v", rel, err)
		}
	}
}

func TestMissingPathInEmptyDirectorySaysEmpty(t *testing.T) {
	code, root := fixture(t)
	if err := os.MkdirAll(filepath.Join(root, "void"), 0o750); err != nil {
		t.Fatal(err)
	}
	_, err := code.ListDir(context.Background(), "void/x")
	if err == nil || err.Error() != `not found: "void/x"; void is empty` {
		t.Fatalf("got %v", err)
	}
}

func TestMissingPathHintLeadsWithNamesNearTheMissingOne(t *testing.T) {
	code, root := fixture(t)
	for _, name := range []string{"charge.js", "charger.js", "index.js", "alpha.js", "Chat.js"} {
		if err := os.WriteFile(filepath.Join(root, "pkg", name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := code.ReadFile(context.Background(), "pkg/charg.js", 1, 1)
	if err == nil || !strings.Contains(err.Error(), "pkg has: charge.js, charger.js, Chat.js, ") {
		t.Fatalf("got %v", err)
	}
}

func TestMissingPathHintSkipsGitAndBoundsTheScan(t *testing.T) {
	code, root := fixture(t)
	_, err := code.ListDir(context.Background(), "nope")
	if err == nil || strings.Contains(err.Error(), ".git") {
		t.Fatalf("got %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "wide"), 0o750); err != nil {
		t.Fatal(err)
	}
	for i := range maxHintScan + 10 {
		if err := os.WriteFile(filepath.Join(root, "wide", fmt.Sprintf("f%04d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err = code.ListDir(context.Background(), "wide/x")
	if err == nil || !strings.HasSuffix(err.Error(), ", and more") {
		t.Fatalf("got %v", err)
	}
}

func TestMissingPathHintStopsOnACancelledContext(t *testing.T) {
	code, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := code.ListDir(ctx, "nope"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestMissingPathHintReachesTheCallerOverHTTP(t *testing.T) {
	c := connect(t)
	res := call3(t, c, "list_dir", map[string]any{"path": "src/pkg"})
	if !res.IsError || !strings.Contains(text(res), `not found: "src/pkg"; the root has: `) {
		t.Fatalf("got %+v", res)
	}
}
