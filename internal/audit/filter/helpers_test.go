package filter

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	testWindow = Window{
		From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
	}
	placeholderPattern = regexp.MustCompile(`\$(\d+)`)
)

const windowPrefix = "occurred_at >= $1 AND occurred_at < $2"

func mustParse(t *testing.T, text string) Query {
	t.Helper()
	q, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return q
}

func assertSafeSQL(t *testing.T, sql string, args []any) {
	t.Helper()
	skeleton := strings.NewReplacer("'simple'", "", `ESCAPE '\'`, "").Replace(sql)
	for _, bad := range []string{"'", `"`, ";", "--", "/*", `\`} {
		if strings.Contains(skeleton, bad) {
			t.Fatalf("SQL contains %q: %s", bad, sql)
		}
	}
	used := map[int]bool{}
	for _, m := range placeholderPattern.FindAllStringSubmatch(sql, -1) {
		n, _ := strconv.Atoi(m[1])
		used[n] = true
	}
	for i := 1; i <= len(args); i++ {
		if !used[i] {
			t.Fatalf("argument $%d is never used: %s", i, sql)
		}
	}
	if len(used) != len(args) {
		t.Fatalf("SQL uses %d placeholders for %d arguments: %s", len(used), len(args), sql)
	}
}

func whereAndArgs(t *testing.T, text string) (string, []any) {
	t.Helper()
	c, err := mustParse(t, text).Rows(testWindow, Page{})
	if err != nil {
		t.Fatalf("Rows(%q): %v", text, err)
	}
	assertSafeSQL(t, c.SQL, c.Args)
	_, after, _ := strings.Cut(c.SQL, " WHERE ")
	after, _, _ = strings.Cut(after, " ORDER BY")
	return strings.TrimPrefix(strings.TrimPrefix(after, windowPrefix), " AND "), c.Args[2 : len(c.Args)-1]
}
