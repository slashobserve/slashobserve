package keygen

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var keyPattern = regexp.MustCompile(`^sok_[A-Za-z0-9_-]{43}\n$`)

func readKey(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a test file under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunWritesAKeyOnlyWhenTheFileIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "loadgen.key")

	outcome, err := Run(path)
	if err != nil || outcome != Written {
		t.Fatalf("first run = %q, %v, want written", outcome, err)
	}
	first := readKey(t, path)
	if !keyPattern.MatchString(first) {
		t.Fatalf("the file holds %d bytes that are not one sok_ key", len(first))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o640 {
		t.Fatalf("mode = %o, want 640", mode)
	}

	outcome, err = Run(path)
	if err != nil || outcome != Kept {
		t.Fatalf("second run = %q, %v, want kept", outcome, err)
	}
	if readKey(t, path) != first {
		t.Fatal("the second run changed the key")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory holds %d entries, want only the key file", len(entries))
	}
}

func TestRunKeepsAnOperatorsOwnFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loadgen.key")
	if err := os.WriteFile(path, []byte("chosen by the operator\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if outcome, err := Run(path); err != nil || outcome != Kept {
		t.Fatalf("got %q, %v, want kept", outcome, err)
	}
	if got := readKey(t, path); got != "chosen by the operator\n" {
		t.Fatalf("file now holds %q", got)
	}
}

func TestRunFailsWhenTheDirectoryIsMissing(t *testing.T) {
	if _, err := Run(filepath.Join(t.TempDir(), "absent", "loadgen.key")); err == nil {
		t.Fatal("got no error, want one")
	}
}

func TestEachRunWritesANewKey(t *testing.T) {
	a, b := filepath.Join(t.TempDir(), "a.key"), filepath.Join(t.TempDir(), "b.key")
	for _, path := range []string{a, b} {
		if _, err := Run(path); err != nil {
			t.Fatal(err)
		}
	}
	if readKey(t, a) == readKey(t, b) {
		t.Fatal("two runs wrote the same key")
	}
}
