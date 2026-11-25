package filter

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRowsPageByKeyset(t *testing.T) {
	q := mustParse(t, "outcome:denied")
	id := uuid.MustParse("0193a1f0-0000-7000-8000-000000000001")
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	c, err := q.Rows(testWindow, Page{After: &Cursor{OccurredAt: at, ID: id.String()}, Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	wantTail := " AND (occurred_at, id) < ($4, $5::uuid) ORDER BY occurred_at DESC, id DESC LIMIT $6"
	if !strings.HasSuffix(c.SQL, wantTail) {
		t.Errorf("SQL tail\n got %s\nwant ...%s", c.SQL, wantTail)
	}
	wantArgs := []any{testWindow.From, testWindow.To, "denied", at, id.String(), 26}
	if !reflect.DeepEqual(c.Args, wantArgs) {
		t.Errorf("args\n got %#v\nwant %#v", c.Args, wantArgs)
	}
	if c.Limit != 25 {
		t.Errorf("limit = %d, want 25 (the SQL fetches one more to show there is a next page)", c.Limit)
	}
	if !strings.HasPrefix(c.SQL, "SELECT id, seq, occurred_at, ") || strings.Contains(c.SQL, "search") || strings.Contains(c.SQL, "xid") {
		t.Errorf("rows must select the event columns without search or xid: %s", c.SQL)
	}
	if !reflect.DeepEqual(c.Columns, eventColumns) {
		t.Errorf("columns = %v", c.Columns)
	}
	assertSafeSQL(t, c.SQL, c.Args)
}

func TestRowsLimits(t *testing.T) {
	plain := mustParse(t, "")
	sized := mustParse(t, `{"size":7}`)
	for _, tc := range []struct {
		name  string
		q     Query
		limit int
		want  int
		fails bool
	}{
		{"default", plain, 0, DefaultRows, false},
		{"asked", plain, 1, 1, false},
		{"ceiling", plain, MaxRows, MaxRows, false},
		{"over the ceiling", plain, MaxRows + 1, 0, true},
		{"negative", plain, -1, 0, true},
		{"size from the body", sized, 0, 7, false},
		{"asked beats the body", sized, 3, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.q.Rows(testWindow, Page{Limit: tc.limit})
			if tc.fails {
				if err == nil {
					t.Fatal("Rows accepted the limit")
				}
				return
			}
			if err != nil || c.Limit != tc.want || c.Args[len(c.Args)-1] != tc.want+1 {
				t.Errorf("limit = %d, args = %#v, err = %v; want %d", c.Limit, c.Args, err, tc.want)
			}
		})
	}
}

func TestAnExplicitSizeOfZeroReturnsNoRows(t *testing.T) {
	for _, text := range []string{`{"size":0}`, `{"size":0,"aggs":{"n":{"cardinality":{"field":"actor_id"}}}}`} {
		q := mustParse(t, text)
		for _, asked := range []int{0, 5} {
			c, err := q.Rows(testWindow, Page{Limit: asked})
			if err != nil || c.Limit != 0 || c.Args[len(c.Args)-1] != 0 {
				t.Errorf("%s asked %d: limit = %d, args = %#v, err = %v", text, asked, c.Limit, c.Args, err)
			}
		}
	}
	q := mustParse(t, `{"query":{"match_all":{}}}`)
	c, err := q.Rows(testWindow, Page{})
	if err != nil || c.Limit != DefaultRows {
		t.Errorf("a query with no size: limit = %d, err = %v", c.Limit, err)
	}
}

func TestRowsRefuseABadWindowOrCursor(t *testing.T) {
	q := mustParse(t, "")
	for _, w := range []Window{{}, {From: testWindow.To, To: testWindow.From}, {From: testWindow.From, To: testWindow.From}} {
		if _, err := q.Rows(w, Page{}); !errors.Is(err, ErrWindow) {
			t.Errorf("Rows(%+v) error = %v", w, err)
		}
	}
	for _, cursor := range []*Cursor{
		{OccurredAt: testWindow.To, ID: "not-a-uuid"},
		{ID: uuid.NewString()},
		{OccurredAt: testWindow.To, ID: "1; DROP TABLE audit_events"},
	} {
		if _, err := q.Rows(testWindow, Page{After: cursor}); err == nil {
			t.Errorf("Rows accepted the cursor %+v", cursor)
		}
	}
}

func TestRowsReadOldestFirstAndSkipAnOffset(t *testing.T) {
	q := mustParse(t, "outcome:denied")
	id := uuid.MustParse("0193a1f0-0000-7000-8000-000000000001")
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	c, err := q.Rows(testWindow, Page{Limit: 10, Offset: 20, Ascending: true, After: &Cursor{OccurredAt: at, ID: id.String()}})
	if err != nil {
		t.Fatal(err)
	}
	wantTail := " AND (occurred_at, id) > ($4, $5::uuid) ORDER BY occurred_at ASC, id ASC LIMIT $6 OFFSET $7"
	if !strings.HasSuffix(c.SQL, wantTail) {
		t.Errorf("SQL tail\n got %s\nwant ...%s", c.SQL, wantTail)
	}
	if got := c.Args[len(c.Args)-2:]; !reflect.DeepEqual(got, []any{11, 20}) {
		t.Errorf("limit and offset args = %#v", got)
	}
	assertSafeSQL(t, c.SQL, c.Args)
	if _, err := q.Rows(testWindow, Page{Offset: -1}); err == nil {
		t.Error("Rows accepted a negative offset")
	}
}

func TestCountStopsAtTheCap(t *testing.T) {
	q := mustParse(t, "outcome:denied")
	capped, err := q.Count(testWindow, 10001)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT count(*) FROM (SELECT 1 FROM audit_events WHERE " + windowPrefix + " AND COALESCE((outcome = $3), false) LIMIT $4) matched"
	if capped.SQL != want || capped.Args[3] != 10001 {
		t.Errorf("capped count\n got %s %#v\nwant %s", capped.SQL, capped.Args, want)
	}
	assertSafeSQL(t, capped.SQL, capped.Args)
	exact, err := q.Count(testWindow, 0)
	if err != nil || strings.Contains(exact.SQL, "LIMIT") || len(exact.Args) != 3 {
		t.Errorf("exact count = %s %#v, %v", exact.SQL, exact.Args, err)
	}
	if _, err := q.Count(testWindow, -1); err == nil {
		t.Error("Count accepted a negative cap")
	}
	if _, err := q.Count(Window{}, 1); !errors.Is(err, ErrWindow) {
		t.Errorf("Count with no window error = %v", err)
	}
}

func TestFollowReadsInCommitOrderAfterAPosition(t *testing.T) {
	q, err := Parse("outcome:denied")
	if err != nil {
		t.Fatal(err)
	}
	c, err := q.Follow(testWindow, Position{Xid: 42, Seq: 7}, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"xid < pg_snapshot_xmin(pg_current_snapshot())", "(xid, seq) > (", "ORDER BY audit_events.xid, audit_events.seq LIMIT"} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("follow SQL lacks %q: %s", want, c.SQL)
		}
	}
	if c.Columns[len(c.Columns)-1] != "xid::text" || !argsContain(c.Args, "42") {
		t.Errorf("columns %v, args %#v", c.Columns, c.Args)
	}
	for _, limit := range []int{0, MaxRows + 1} {
		if _, err := q.Follow(testWindow, Position{}, limit); err == nil {
			t.Errorf("a follow of %d rows was compiled", limit)
		}
	}
}

func TestFacets(t *testing.T) {
	q := mustParse(t, "outcome:denied")
	c, err := q.Facets(testWindow, "actor", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT actor_name AS value, count(*) AS n FROM audit_events WHERE occurred_at >= $1 AND occurred_at < $2 AND COALESCE((outcome = $3), false) AND actor_name IS NOT NULL GROUP BY 1 ORDER BY n DESC, value ASC LIMIT $4"
	if c.SQL != want {
		t.Errorf("SQL\n got %s\nwant %s", c.SQL, want)
	}
	if !reflect.DeepEqual(c.Args, []any{testWindow.From, testWindow.To, "denied", DefaultFacets}) || c.Limit != DefaultFacets {
		t.Errorf("args = %#v, limit = %d", c.Args, c.Limit)
	}
	assertSafeSQL(t, c.SQL, c.Args)
	for field, expr := range map[string]string{"ip": "host(client_ip) AS value", "status": "status::text AS value", "run": "run_id::text AS value", "occurred_at": "occurred_at::text AS value"} {
		c, err := q.Facets(testWindow, field, MaxFacets)
		if err != nil || !strings.Contains(c.SQL, expr) || c.Args[len(c.Args)-1] != MaxFacets {
			t.Errorf("facet on %s = %s %#v %v", field, c.SQL, c.Args, err)
		}
	}
	for _, field := range []string{"target", "nope", "", "1; DROP TABLE x"} {
		if _, err := q.Facets(testWindow, field, 5); err == nil {
			t.Errorf("facet on %q compiled", field)
		}
	}
	if _, err := q.Facets(testWindow, "origin", MaxFacets+1); err == nil {
		t.Error("a facet limit over the ceiling compiled")
	}
	if _, err := q.Facets(Window{}, "origin", 5); !errors.Is(err, ErrWindow) {
		t.Errorf("empty window error = %v", err)
	}
}

func TestInputNeverReachesTheSQLText(t *testing.T) {
	payloads := []string{
		`zz'; DROP TABLE audit_events; --`,
		`zz" OR 1=1 --`,
		`zz\'); DELETE FROM users; --`,
		`zz & !yy | (xx) <-> ww:*`,
		"zz$$ ; $1 $2",
		"zz\n/* xx */",
		"zz%_",
	}
	for _, p := range payloads {
		quoted := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(p) + `"`
		jsonText, _ := jsonString(p)
		for _, text := range []string{
			"action:" + quoted,
			"not actor:" + quoted,
			"target:" + quoted,
			"outcome:denied or " + quoted,
			quoted,
			"route:" + quoted + "*",
			`{"query":{"term":{"action":` + jsonText + `}}}`,
			`{"query":{"match_phrase":{"actor_name":` + jsonText + `}}}`,
			`{"query":{"terms":{"route":[` + jsonText + `]}}}`,
			`{"query":{"wildcard":{"tool":` + jsonText + `}}}`,
			`{"query":{"multi_match":{"query":` + jsonText + `}}}`,
			`{"query":{"range":{"actor_name":{"gte":` + jsonText + `}}}}`,
			"lucene: action:" + quoted,
			"lucene: -actor:" + quoted,
			"lucene: " + quoted,
			"lucene: action:" + escapeLucene(p),
			"lucene: " + escapeLucene(p) + "*",
			"lucene: actor_name:[" + quoted + " TO *]",
			"lucene: actor_name:>" + quoted,
			"lucene: outcome:denied OR actor:(" + quoted + " " + escapeLucene(p) + ")",
		} {
			q, err := Parse(text)
			if err != nil {
				continue
			}
			for name, compile := range map[string]func() (Compiled, error){
				"rows":   func() (Compiled, error) { return q.Rows(testWindow, Page{}) },
				"facets": func() (Compiled, error) { return q.Facets(testWindow, "origin", 5) },
				"follow": func() (Compiled, error) { return q.Follow(testWindow, Position{Xid: 7, Seq: 9}, 5) },
			} {
				c, err := compile()
				if err != nil {
					t.Fatalf("%s of %q: %v", name, text, err)
				}
				assertSafeSQL(t, c.SQL, c.Args)
				if strings.Contains(c.SQL, "zz") || strings.Contains(c.SQL, "DROP") {
					t.Errorf("%s of %q leaks input into SQL: %s", name, text, c.SQL)
				}
				if !argsContain(c.Args, "zz") {
					t.Errorf("%s of %q: arguments do not carry the input: %#v", name, text, c.Args)
				}
			}
		}
	}
}

func escapeLucene(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('\\')
		b.WriteByte(c)
	}
	return b.String()
}

func jsonString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func argsContain(args []any, sub string) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestFieldNamesComeOnlyFromTheFixedList(t *testing.T) {
	for _, text := range []string{
		`{"query":{"term":{"action; DROP TABLE x":"a"}}}`,
		`{"query":{"exists":{"field":"actor_name; --"}}}`,
		`{"query":{"term":{"search":"a"}}}`,
		`{"query":{"term":{"data":"a"}}}`,
		`{"query":{"term":{"xid":"a"}}}`,
		`{"aggs":{"a":{"terms":{"field":"search"}}}}`,
		`{"aggs":{"a":{"terms":{"field":"actor_name; --"}}}}`,
		`{"aggs":{"a":{"max":{"field":"seq"}}}}`,
		"search:x",
	} {
		if _, err := Parse(text); err == nil {
			t.Errorf("Parse(%q) accepted a name outside the field list", text)
		}
	}
}

func TestFieldsListIsConsistent(t *testing.T) {
	columns := map[string]bool{}
	for _, c := range eventColumns {
		columns[c] = true
	}
	seen := map[string]bool{}
	for _, f := range Fields() {
		for _, name := range append([]string{f.Name}, f.Aliases...) {
			if seen[name] {
				t.Errorf("field name %q appears twice", name)
			}
			seen[name] = true
		}
		if f.Description == "" || f.Type == "" {
			t.Errorf("field %s lacks a description or type", f.Name)
		}
		if f.Type == TypeEnum && len(f.Values) == 0 {
			t.Errorf("enum %s lists no values", f.Name)
		}
		if f.Type != TypeTarget && !columns[f.column] {
			t.Errorf("field %s names column %q that rows do not select", f.Name, f.column)
		}
	}
	for _, alias := range []string{"actor", "run", "request", "datasource", "ip", "duration", "occurred_at", "target"} {
		if !seen[alias] {
			t.Errorf("name %q is missing", alias)
		}
	}
	first := Fields()
	first[0].Name = "changed"
	first[2].Values[0] = "changed"
	if again := Fields(); again[0].Name == "changed" || again[2].Values[0] == "changed" {
		t.Error("Fields exposes its internal list")
	}
}

func TestEveryFieldTakesAValueOfItsType(t *testing.T) {
	samples := map[Type]string{
		TypeText:   "zz",
		TypeInt:    "7",
		TypeUUID:   "5b1e0a52-0b7c-4f08-9b1f-6a1d3d6a9a10",
		TypeIP:     "10.0.0.0/8",
		TypeDate:   "2026-11-01T10:00:00Z",
		TypeTarget: "dashboard/1",
	}
	for _, f := range Fields() {
		sample := samples[f.Type]
		if f.Type == TypeEnum {
			sample = f.Values[0]
		}
		q, err := Parse(f.Name + `:"` + sample + `"`)
		if err != nil {
			t.Errorf("%s: %v", f.Name, err)
			continue
		}
		c, err := q.Rows(testWindow, Page{})
		if err != nil {
			t.Errorf("%s rows: %v", f.Name, err)
			continue
		}
		assertSafeSQL(t, c.SQL, c.Args)
		if !f.groupable() {
			continue
		}
		if _, err := q.Facets(testWindow, f.Name, 5); err != nil {
			t.Errorf("%s facets: %v", f.Name, err)
		}
	}
}

func TestEveryAliasMapsToItsField(t *testing.T) {
	for alias, column := range map[string]string{
		"actor": "actor_name", "run": "run_id", "request": "request_id", "datasource": "datasource_uid", "ip": "client_ip", "duration": "duration_ms",
		"actor.name": "actor_name", "actor.id": "actor_id", "actor.kind": "actor_kind", "actor.role": "actor_role", "actor.email": "actor_id",
		"target.kind": "target_kind", "target.id": "target_id", "target.name": "target_name",
	} {
		where, _ := whereAndArgs(t, alias+":*")
		if where != "("+column+" IS NOT NULL)" {
			t.Errorf("%s:* compiled to %s", alias, where)
		}
	}
}

func TestLuceneQueriesFeedEveryShape(t *testing.T) {
	q := mustParse(t, "lucene: outcome:(denied OR failed) AND NOT actor_name:Riya")
	rows, err := q.Rows(testWindow, Page{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	wantWhere := "((COALESCE((outcome = $3), false) OR COALESCE((outcome = $4), false))) AND NOT COALESCE((actor_name = $5), false)"
	if !strings.Contains(rows.SQL, wantWhere) || !reflect.DeepEqual(rows.Args[2:5], []any{"denied", "failed", "Riya"}) {
		t.Errorf("rows = %s %#v", rows.SQL, rows.Args)
	}
	facets, err := q.Facets(testWindow, "origin", 5)
	if err != nil || !strings.Contains(facets.SQL, wantWhere) {
		t.Errorf("facets = %s %v", facets.SQL, err)
	}
	hist, err := q.Histogram(testWindow, time.Minute)
	if err != nil || !strings.Contains(hist.SQL, wantWhere) || !hist.HasTimeBucket() {
		t.Errorf("histogram = %s %v", hist.SQL, err)
	}
	for _, c := range []struct {
		sql  string
		args []any
	}{{rows.SQL, rows.Args}, {facets.SQL, facets.Args}, {hist.SQL, hist.Args}} {
		assertSafeSQL(t, c.sql, c.args)
	}
	if _, err := q.Aggregate(testWindow); err == nil {
		t.Error("a Lucene query has no aggs to aggregate")
	}
}

func TestDottedNamesWorkInEveryQueryLanguage(t *testing.T) {
	const id = "5b1e0a52-0b7c-4f08-9b1f-6a1d3d6a9a10"
	for _, text := range []string{
		`actor.name:"Ada" and target.kind:dashboard and target.id:abc and target.name:Checkout and actor.role:admin and actor.kind:user`,
		`lucene: actor.name:Ada AND target.kind:dashboard AND actor.id:` + id,
		`{"query":{"bool":{"must":[{"term":{"actor.name":"Ada"}},{"term":{"target.kind":"dashboard"}},{"term":{"actor.id":"` + id + `"}}]}}}`,
	} {
		if _, err := mustParse(t, text).Rows(testWindow, Page{}); err != nil {
			t.Errorf("Rows(%q): %v", text, err)
		}
	}
}

func TestAnActorIsFoundByEmailOrByName(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{`actor.email:"admin@example.com"`, "COALESCE((actor_id IN (SELECT id FROM users WHERE lower(email) = lower($3))), false)"},
		{`actor:"admin@example.com"`, "COALESCE(((actor_name = $3 OR actor_id IN (SELECT id FROM users WHERE lower(email) = lower($3)))), false)"},
		{`actor.name:"admin@example.com"`, "COALESCE((actor_name = $3), false)"},
		{`actor_name:"admin@example.com"`, "COALESCE((actor_name = $3), false)"},
		{`actor.email:admin*`, "COALESCE(actor_id IN (SELECT id FROM users WHERE email ILIKE $3 ESCAPE '\\'), false)"},
		{`actor:admin*`, "COALESCE((actor_name ILIKE $3 ESCAPE '\\' OR actor_id IN (SELECT id FROM users WHERE email ILIKE $3 ESCAPE '\\')), false)"},
	} {
		where, args := whereAndArgs(t, tc.text)
		if !strings.Contains(where, tc.want) || len(args) != 1 {
			t.Errorf("%s compiled to %s with %v, want %s", tc.text, where, args, tc.want)
		}
	}
}

func TestAnUnknownFieldNamesTheNearestValidOnes(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"actor.emial:x", `unknown field "actor.emial"; did you mean "actor.email"?`},
		{"acter.name:x", `did you mean "actor.name"`},
		{"actor.zzz:x", `unknown field "actor.zzz"; did you mean `},
		{"target.zzzz:x", `"target.`},
		{"nothingalike:x", `unknown field "nothingalike"`},
	} {
		_, err := Parse(tc.text)
		var parse *Error
		if !errors.As(err, &parse) || !strings.Contains(parse.Msg, tc.want) {
			t.Errorf("Parse(%q) = %v, want it to say %q", tc.text, err, tc.want)
		}
	}
	_, err := Parse("actor.zzz:x")
	var parse *Error
	if errors.As(err, &parse) && strings.Count(parse.Msg, `"actor.`) < 3 {
		t.Errorf("an unknown actor field names %q, want three nearest actor fields", parse.Msg)
	}
}
