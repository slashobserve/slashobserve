package specfiles_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/specfiles"
)

func TestDirNameEscapesOnlyPercentAndSlashAndRoundTrips(t *testing.T) {
	cases := []struct{ title, dir string }{
		{"Payments", "Payments"},
		{"Team / Payments", "Team %2F Payments"},
		{"100% uptime", "100%25 uptime"},
		{"a%2Fb", "a%252Fb"},
		{"%25/%", "%2525%2F%25"},
		{"Größe & Café ✓", "Größe & Café ✓"},
		{"  spaced  out  ", "  spaced  out  "},
		{"", ""},
	}
	for _, c := range cases {
		if got := specfiles.DirName(c.title); got != c.dir {
			t.Errorf("DirName(%q) = %q, want %q", c.title, got, c.dir)
		}
		if got := specfiles.FolderTitle(c.dir); got != c.title {
			t.Errorf("FolderTitle(%q) = %q, want %q", c.dir, got, c.title)
		}
	}
}

func TestFolderOfIsTheDecodedFirstSegmentAndEmptyAtTheRoot(t *testing.T) {
	cases := []struct{ path, folder string }{
		{"overview.json", ""},
		{"Payments/checkout.json", "Payments"},
		{"Team %2F Payments/checkout.json", "Team / Payments"},
		{"Ops/nested/deep.json", "Ops"},
	}
	for _, c := range cases {
		if got := specfiles.FolderOf(c.path); got != c.folder {
			t.Errorf("FolderOf(%q) = %q, want %q", c.path, got, c.folder)
		}
	}
}

func TestFileNameAddsTheJSONExtension(t *testing.T) {
	if got := specfiles.FileName("checkout-health"); got != "checkout-health.json" {
		t.Errorf("FileName = %q", got)
	}
}

func TestScanKeepsJSONFilesSkipsDotEntriesAndDecodesFolders(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"root.json",
		"notes.txt",
		".hidden.json",
		".git/config.json",
		"Payments/checkout.json",
		"Team %2F Payments/api.json",
		"Payments/.draft.json",
	} {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := specfiles.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []specfiles.File{
		{Path: "Payments/checkout.json", Folder: "Payments"},
		{Path: "Team %2F Payments/api.json", Folder: "Team / Payments"},
		{Path: "root.json", Folder: ""},
	}
	if !slices.Equal(files, want) {
		t.Errorf("Scan = %+v, want %+v", files, want)
	}
}

func TestScanFailsOnAMissingDirectory(t *testing.T) {
	if _, err := specfiles.Scan(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Scan of a missing directory succeeded")
	}
}
