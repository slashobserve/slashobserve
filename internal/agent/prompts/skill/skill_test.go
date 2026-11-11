package skill

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var skillName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

func frontMatter(t *testing.T) map[string]string {
	t.Helper()
	rest, ok := strings.CutPrefix(string(File()), "---\n")
	if !ok {
		t.Fatal("the skill file does not start with front matter")
	}
	block, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		t.Fatal("the skill file front matter is not closed")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("front matter line %q is not key: value", line)
		}
		fields[key] = value
	}
	return fields
}

func TestFrontMatterFollowsTheAgentSkillsRules(t *testing.T) {
	fields := frontMatter(t)
	if fields["name"] != Name {
		t.Errorf("name is %q, want %q", fields["name"], Name)
	}
	if !skillName.MatchString(fields["name"]) {
		t.Errorf("name %q does not match %s", fields["name"], skillName)
	}
	if _, err := os.Stat(fields["name"] + "/SKILL.md"); err != nil {
		t.Errorf("the skill file is not in a directory named after the skill: %v", err)
	}
	if n := utf8.RuneCountInString(fields["description"]); n < 1 || n > 1024 {
		t.Errorf("description is %d characters, want 1 to 1024", n)
	}
	if len(fields) != 2 {
		t.Errorf("front matter has %d fields, want name and description only", len(fields))
	}
}

func TestBodyStartsAtTheGuideHeading(t *testing.T) {
	if !strings.HasPrefix(Body(), "# /observe\n") {
		t.Errorf("body starts with %q, want the /observe heading", Body()[:20])
	}
	if strings.Contains(Body(), "description:") {
		t.Error("body still holds front matter")
	}
}

func TestPromptAppendsTheTrimmedAsk(t *testing.T) {
	cases := []struct {
		name, ask, want string
	}{
		{"empty ask", "", Body()},
		{"blank ask", " \n\t ", Body()},
		{"ask with padding", "  checkout p99  ", Body() + "\n\n## The ask\n\ncheckout p99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Prompt(tc.ask); got != tc.want {
				t.Errorf("Prompt(%q) = %q, want %q", tc.ask, got, tc.want)
			}
		})
	}
	if !strings.HasSuffix(Prompt("  checkout p99  "), "## The ask\n\ncheckout p99") {
		t.Error("the ask is not the last section")
	}
}

func TestFileReturnsACopy(t *testing.T) {
	first := File()
	want := string(first)
	first[0] = 'X'
	if got := string(File()); got != want {
		t.Error("changing the returned bytes changed the embedded file")
	}
}

func TestBodyOfRejectsFilesWithoutFrontMatter(t *testing.T) {
	cases := map[string]string{
		"no front matter": "# /observe\n",
		"not closed":      "---\nname: observe\n# /observe\n",
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("bodyOf did not panic")
				}
			}()
			bodyOf([]byte(file))
		})
	}
}
