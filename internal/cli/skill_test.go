package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts/skill"
)

func writtenFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}

func assertSkillFile(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path) //nolint:gosec // a path the test just installed into a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(skill.File()) {
		t.Errorf("%s is not the embedded skill", path)
	}
}

func TestSkillInstallWritesTheEmbeddedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stale := filepath.Join(home, ".claude", "skills", "observe", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("an older skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI("skill", "install")

	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	agents := filepath.Join(home, ".agents", "skills", "observe", "SKILL.md")
	if want := "Installed " + agents + "\nInstalled " + stale + "\n"; stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
	assertSkillFile(t, agents)
	assertSkillFile(t, stale)
	if got := writtenFiles(t, home); !slices.Equal(got, []string{agents, stale}) {
		t.Errorf("files under home: %v", got)
	}
}

func TestSkillInstallWithDirWritesOnlyThatRoot(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, code := runCLI("skill", "install", "--dir", dir)

	want := filepath.Join(dir, "observe", "SKILL.md")
	if code != 0 || stdout != "Installed "+want+"\n" {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	assertSkillFile(t, want)
	if got := writtenFiles(t, home); len(got) != 0 {
		t.Errorf("--dir also wrote under home: %v", got)
	}
}

func TestSkillInstallNamesThePathItCannotWrite(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(blocker, "skills")

	stdout, stderr, code := runCLI("skill", "install", "--dir", dir)

	if code != 1 || stdout != "" || !strings.Contains(stderr, dir) {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestSkillInstallFailsWithoutAHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")

	_, stderr, code := runCLI("skill", "install")

	if code != 1 || !strings.Contains(stderr, "pass --dir") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestSkillCommandsRefuseUnknownWords(t *testing.T) {
	cases := [][]string{{"skill", "install", "extra"}, {"skill", "update"}}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			_, stderr, code := runCLI(args...)
			if code != 2 || stderr == "" {
				t.Errorf("exit %d, stderr %q", code, stderr)
			}
		})
	}
}
