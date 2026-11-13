package search

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSearchFollowsEveryChangeAtOnce(t *testing.T) {
	t.Run("a dashboard moved into a restricted folder takes its annotations and comments but not an observation filed beside it", func(t *testing.T) {
		r := newRig(t)
		dashboard := r.dashboard(t, "quokka dashboard", mustUUID(t, r.text(t, `SELECT general_folder_id()::text`)), "")
		r.finding(t, findingIn{title: "quokka finding", markdown: "quokka", dashboard: dashboard})
		annotation := r.annotation(t, dashboard, "quokka annotation")
		r.comment(t, KindDashboard, dashboard, "quokka comment")
		r.comment(t, KindAnnotation, annotation, "quokka reply")
		if got := len(r.search(t, editor, "quokka")); got != 5 {
			t.Fatalf("before the move the editor finds %d results, want 5", got)
		}

		r.db.Exec(t, `UPDATE dashboards SET folder_id = $1 WHERE id = $2::uuid`, r.restricted(t, "Payroll"), dashboard)

		if got := r.search(t, editor, "quokka"); len(got) != 1 || got[0].Kind != KindObservation {
			t.Fatalf("after the move the editor finds %v, want only the observation", titles(got))
		}
		if got := len(r.search(t, payroll, "quokka")); got != 5 {
			t.Fatalf("the folder's viewer finds %d results, want 5", got)
		}
	})

	t.Run("an observation moved into a shared folder is found by that folder's viewers", func(t *testing.T) {
		r := newRig(t)
		observation := r.finding(t, findingIn{title: "quokka finding", markdown: "quokka", folder: r.restricted(t, "Payroll")})
		if got := r.search(t, editor, "quokka"); len(got) != 0 {
			t.Fatalf("before the move the editor finds %v", titles(got))
		}

		r.db.Exec(t, `UPDATE observations SET folder_id = $1 WHERE id = $2::uuid`, r.folder(t, "Incidents"), observation)

		got := r.search(t, editor, "quokka")
		if len(got) != 1 || got[0].Folder == nil || got[0].Folder.Title != "Incidents" {
			t.Fatalf("after the move the editor finds %+v", got)
		}
	})

	t.Run("a deleted dashboard takes its annotations and comments out of the results and leaves the observation", func(t *testing.T) {
		r := newRig(t)
		dashboard := r.dashboard(t, "quokka dashboard", mustUUID(t, r.text(t, `SELECT general_folder_id()::text`)), "")
		r.finding(t, findingIn{title: "quokka finding", markdown: "quokka", dashboard: dashboard})
		r.annotation(t, dashboard, "quokka annotation")
		r.comment(t, KindDashboard, dashboard, "quokka comment")

		r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1::uuid`, dashboard)

		if got := r.search(t, admin, "quokka"); len(got) != 1 || got[0].Kind != KindObservation {
			t.Fatalf("found %v after the delete, want only the observation", titles(got))
		}
	})

	t.Run("an edited message is found by its new words and no longer by its old ones", func(t *testing.T) {
		r := newRig(t)
		dashboard := r.dashboard(t, "Plain", mustUUID(t, r.text(t, `SELECT general_folder_id()::text`)), "")
		_, message := r.comment(t, KindDashboard, dashboard, "the quokka is loose")

		r.db.Exec(t, `UPDATE thread_messages SET body = 'the numbat is loose', edited_at = now() WHERE id = $1::uuid`, message)

		if got := r.search(t, admin, "quokka"); len(got) != 0 {
			t.Fatalf("the old word still finds %v", titles(got))
		}
		if got := r.search(t, admin, "numbat"); len(got) != 1 || got[0].Kind != KindComment {
			t.Fatalf("the new word finds %+v", got)
		}
	})

	t.Run("a word only in an old dashboard version finds nothing", func(t *testing.T) {
		r := newRig(t)
		dashboard := r.dashboard(t, "Plain", mustUUID(t, r.text(t, `SELECT general_folder_id()::text`)), "walrus colony")
		r.version(t, dashboard, 2, "Plain", "meerkat colony")
		r.db.Exec(t, `UPDATE dashboards SET latest_version = 2 WHERE id = $1::uuid`, dashboard)

		if got := r.search(t, admin, "walrus"); len(got) != 0 {
			t.Fatalf("an old version finds %v", titles(got))
		}
		if got := r.search(t, admin, "meerkat"); len(got) != 1 {
			t.Fatalf("the latest version finds %v", titles(got))
		}
	})
}

func TestSearchRanksTitlesAboveBodiesAndNewerAboveOlder(t *testing.T) {
	t.Run("a title match beats a body match", func(t *testing.T) {
		r := newRig(t)
		r.dashboard(t, "Birds", mustUUID(t, r.text(t, `SELECT general_folder_id()::text`)), "a kestrel nest")
		r.dashboard(t, "Kestrel nest", mustUUID(t, r.text(t, `SELECT general_folder_id()::text`)), "")

		got := titles(r.search(t, admin, "kestrel"))

		if !slices.Equal(got, []string{"Kestrel nest", "Birds"}) {
			t.Fatalf("order = %v", got)
		}
	})

	t.Run("at equal text the newer object comes first", func(t *testing.T) {
		r := newRig(t)
		r.finding(t, findingIn{title: "old finding", markdown: "kestrel sighting", ageDays: 90})
		r.finding(t, findingIn{title: "new finding", markdown: "kestrel sighting"})

		got := titles(r.search(t, admin, "kestrel"))

		if !slices.Equal(got, []string{"new finding", "old finding"}) {
			t.Fatalf("order = %v", got)
		}
	})

	t.Run("a prefix-only match has a rank above zero", func(t *testing.T) {
		r := newRig(t)
		r.finding(t, findingIn{title: "kestrel", markdown: "nest"})
		var rank float64
		err := r.db.SQLDB(t).QueryRowContext(t.Context(),
			`SELECT ts_rank_cd(observation_search_vector(title, summary), search_query('kest', 'kest'), 1) FROM observations`).Scan(&rank)
		if err != nil || rank <= 0 {
			t.Fatalf("rank %v, %v", rank, err)
		}
		if got := r.search(t, admin, "kest"); len(got) != 1 {
			t.Fatalf("a prefix finds %v", titles(got))
		}
	})
}

func TestSearchQuerySyntax(t *testing.T) {
	r := newRig(t)
	for title, markdown := range map[string]string{
		"one":   "alpha beta",
		"two":   "alpha gamma",
		"three": "beta alpha",
		"four":  "an error occurred",
	} {
		r.finding(t, findingIn{title: title, markdown: markdown})
	}
	cases := []struct {
		name string
		q    string
		want []string
	}{
		{"words are ANDed", "alpha beta", []string{"one", "three"}},
		{"a phrase keeps the order", `"alpha beta"`, []string{"one"}},
		{"or gives either", "gamma or beta", []string{"one", "three", "two"}},
		{"a minus term excludes", "alpha -beta", []string{"two"}},
		{"a stem matches its plural", "errors", []string{"four"}},
		{"the last word matches as a prefix", "alpha bet", []string{"one", "three"}},
		{"a prefix alone", "err", []string{"four"}},
		{"a finished word does not match as a prefix", "err ", nil},
		{"case is ignored", "ALPHA GAMMA", []string{"two"}},
		{"punctuation splits words", "alpha,gamma", []string{"two"}},
		{"a stop word still matches exactly", "an error", []string{"four"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := titles(r.search(t, admin, tt.q))
			slices.Sort(got)
			if !slices.Equal(got, tt.want) && (len(got) != 0 || len(tt.want) != 0) {
				t.Fatalf("%q finds %v, want %v", tt.q, got, tt.want)
			}
		})
	}
}

func TestSearchKindFilter(t *testing.T) {
	w := buildWorld(t)
	cases := []struct {
		kinds []Kind
		want  int
	}{
		{[]Kind{KindDashboard}, 2},
		{[]Kind{KindObservation}, 4},
		{[]Kind{KindAnnotation}, 2},
		{[]Kind{KindComment}, 8},
		{[]Kind{KindSavedSearch}, 1},
		{[]Kind{KindDashboard, KindSavedSearch}, 3},
		{nil, 17},
	}
	for _, tt := range cases {
		if got := w.search(t, admin, "zebra", tt.kinds...); len(got) != tt.want {
			t.Errorf("kinds %v find %d, want %d", tt.kinds, len(got), tt.want)
		}
	}
}

func TestSearchPagesAreDisjointOrderedAndStopAtTheCap(t *testing.T) {
	r := newRig(t)
	r.db.Exec(t, `INSERT INTO annotations (time_from, text, created_at)
SELECT now(), 'meerkat ' || n, now() - make_interval(mins => n) FROM generate_series(1, 230) n`)

	t.Run("the pages add up to the first big page", func(t *testing.T) {
		big, err := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		var small []Result
		cursor := ""
		for len(small) < 49 {
			page, err := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 7, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			small = append(small, page.Results...)
			cursor = page.Next
		}
		for i, got := range small[:49] {
			if got.ID != big.Results[i].ID {
				t.Fatalf("result %d differs: %s against %s", i, got.ID, big.Results[i].ID)
			}
		}
	})

	t.Run("the same cursor gives the same page", func(t *testing.T) {
		first, err := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 5})
		if err != nil || first.Next == "" {
			t.Fatalf("%v, next %q", err, first.Next)
		}
		r.db.Exec(t, `INSERT INTO annotations (time_from, text) VALUES (now(), 'meerkat newest')`)
		a, _ := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 5, Cursor: first.Next})
		b, _ := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 5, Cursor: first.Next})
		if !slices.Equal(titles(a.Results), titles(b.Results)) || len(a.Results) != 5 {
			t.Fatalf("pages differ: %v and %v", titles(a.Results), titles(b.Results))
		}
	})

	t.Run("results stop at 200 and the last page has no cursor", func(t *testing.T) {
		seen := map[string]bool{}
		cursor := ""
		pages := 0
		for {
			page, err := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: MaxLimit, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			for _, res := range page.Results {
				if seen[res.ID] {
					t.Fatalf("%s is on two pages", res.ID)
				}
				seen[res.ID] = true
			}
			pages++
			if page.Next == "" {
				break
			}
			cursor = page.Next
		}
		if len(seen) != MaxResults || pages != 4 {
			t.Fatalf("%d results in %d pages, want %d in 4", len(seen), pages, MaxResults)
		}
	})

	t.Run("a page that would cross the cap is cut at it", func(t *testing.T) {
		first, err := r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		cursor := first.Next
		var last Page
		for range 3 {
			if last, err = r.service.Search(t.Context(), admin, Request{Q: "meerkat", Limit: 50, Cursor: cursor}); err != nil {
				t.Fatal(err)
			}
			cursor = last.Next
		}
		if len(last.Results) != 50 || last.Next != "" {
			t.Fatalf("last page has %d results and cursor %q", len(last.Results), last.Next)
		}
	})
}

func TestSearchRejectsBadInput(t *testing.T) {
	r := newRig(t)
	var invalid *InvalidQueryError
	if _, err := r.service.Search(t.Context(), admin, Request{Q: "--"}); !errors.As(err, &invalid) {
		t.Errorf("a query with no word: %v", err)
	}
	if _, err := r.service.Search(t.Context(), admin, Request{Q: "zebra", Cursor: "garbage"}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a bad cursor: %v", err)
	}
}

func TestSearchSnippets(t *testing.T) {
	r := newRig(t)
	general := mustUUID(t, r.text(t, `SELECT general_folder_id()::text`))
	dashboard := r.dashboard(t, "Plain", general, "")
	r.finding(t, findingIn{title: "crossing", markdown: "The zebra crossing was blocked by a pelican"})
	r.finding(t, findingIn{title: "osprey", markdown: "plain narrative text without the word"})
	r.finding(t, findingIn{title: "stray", markdown: "okapi \x02fake\x03 text"})
	r.comment(t, KindDashboard, dashboard, "ask @[Grace Hopper](user:11111111-1111-4111-8111-111111111111) about the gazelle")
	r.savedSearch(t, "traces", "hyena traces", nil, general)

	t.Run("the matched word is a match segment", func(t *testing.T) {
		got := r.search(t, admin, "zebra")
		if len(got) != 1 || !slices.Contains(got[0].Snippet, Segment{Text: "zebra", Match: true}) {
			t.Fatalf("snippet %v", got)
		}
	})
	t.Run("a mention shows as the name and never as its token", func(t *testing.T) {
		got := r.search(t, admin, "gazelle")
		text := PlainText(got[0].Snippet)
		if len(got) != 1 || !strings.Contains(text, "Grace Hopper") || strings.Contains(text, "user:") || strings.Contains(text, "11111111") {
			t.Fatalf("snippet %q", text)
		}
	})
	t.Run("a title-only match shows the start of the text with no match", func(t *testing.T) {
		got := r.search(t, admin, "osprey")
		if len(got) != 1 || PlainText(got[0].Snippet) != "plain narrative text without the word" || slices.ContainsFunc(got[0].Snippet, func(s Segment) bool { return s.Match }) {
			t.Fatalf("snippet %v", got)
		}
	})
	t.Run("an object with no text has an empty snippet", func(t *testing.T) {
		got := r.search(t, admin, "hyena")
		if len(got) != 1 || got[0].Snippet == nil || len(got[0].Snippet) != 0 {
			t.Fatalf("snippet %#v", got)
		}
	})
	t.Run("marker characters in the text never make a match", func(t *testing.T) {
		got := r.search(t, admin, "okapi")
		for _, s := range got[0].Snippet {
			if s.Match && s.Text != "okapi" {
				t.Fatalf("snippet %v marks %q", got[0].Snippet, s.Text)
			}
		}
		if text := PlainText(got[0].Snippet); !strings.Contains(text, "fake") {
			t.Fatalf("snippet %q lost the text", text)
		}
	})
}

func TestSearchResultsCarryTheirPlaceAndAddress(t *testing.T) {
	r := newRig(t)
	general := mustUUID(t, r.text(t, `SELECT general_folder_id()::text`))
	dashboard := r.dashboard(t, "Checkout overview", general, "")
	global := r.annotation(t, "", "heron global")
	onDashboard := r.annotation(t, dashboard, "heron on dashboard")
	finding := r.finding(t, findingIn{title: "heron finding", markdown: "text", dashboard: dashboard})
	thread, message := r.comment(t, KindObservation, finding, "heron comment")
	keyed := r.finding(t, findingIn{title: "heron keyed", markdown: "text", by: keyS})
	logs := `{app="x"} |= "heron"`
	saved := r.savedSearch(t, "logs", "heron logs", &logs, general)
	r.db.Exec(t, `UPDATE annotations SET time_from = '2026-11-01T10:00:00Z', time_to = '2026-11-01T10:30:00Z' WHERE id = $1::uuid`, onDashboard)

	byID := map[string]Result{}
	for _, res := range r.search(t, admin, "heron") {
		byID[res.ID] = res
	}

	cases := []struct {
		name    string
		id      string
		url     string
		folder  string
		target  *Target
		thread  string
		kind    Kind
		hasPath bool
	}{
		{"a global annotation has no page", global, "", "General", nil, "", KindAnnotation, false},
		{"an annotation on a dashboard opens its window", onDashboard,
			"https://observe.example.com/d/" + dashboard + "/checkout-overview?from=1793526300000&to=1793529900000", "General", nil, "", KindAnnotation, true},
		{"a converted finding opens its observation", finding, "https://observe.example.com/o/" + finding, "General", nil, "", KindObservation, true},
		{"a comment points at what it is on", message, "https://observe.example.com/o/" + finding, "General",
			&Target{Kind: KindObservation, ID: finding, Title: "heron finding"}, thread, KindComment, true},
		{"an observation saved by a key is filed in General", keyed, "https://observe.example.com/o/" + keyed, "General", nil, "", KindObservation, true},
		{"a saved search opens its page", saved, "https://observe.example.com/logs?ds=loki&from=now-1h&q=%7Bapp%3D%22x%22%7D+%7C%3D+%22heron%22&to=now", "General", nil, "", KindSavedSearch, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := byID[tt.id]
			if !ok {
				t.Fatalf("%s is not in the results", tt.id)
			}
			if got.Kind != tt.kind || got.URL != tt.url || got.ThreadID != tt.thread {
				t.Fatalf("kind %s url %q thread %q, want %s %q %q", got.Kind, got.URL, got.ThreadID, tt.kind, tt.url, tt.thread)
			}
			if tt.folder == "" && got.Folder != nil || tt.folder != "" && (got.Folder == nil || got.Folder.Title != tt.folder) {
				t.Fatalf("folder %+v, want %q", got.Folder, tt.folder)
			}
			if (tt.target == nil) != (got.Target == nil) || tt.target != nil && *got.Target != *tt.target {
				t.Fatalf("target %+v, want %+v", got.Target, tt.target)
			}
		})
	}
}

func TestAnObservationIsReturnedOnceWhateverKindsAreAsked(t *testing.T) {
	r := newRig(t)
	converted := r.finding(t, findingIn{title: "marmot burrow", markdown: "marmot tunnels under the road\nsecond line", summary: "marmot tunnels under the road"})
	r.finding(t, findingIn{title: "other", markdown: "nothing relevant"})

	cases := []struct {
		name  string
		kinds []Kind
	}{
		{"no kinds", nil},
		{"observation", []Kind{KindObservation}},
		{"observation twice", []Kind{KindObservation, KindObservation}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := r.search(t, admin, "marmot", tt.kinds...)
			if len(got) != 1 || got[0].Kind != KindObservation || got[0].ID != converted || got[0].URL != "https://observe.example.com/o/"+converted {
				t.Fatalf("results %+v, want the observation %s once", got, converted)
			}
		})
	}

}

func TestOnlyTheListedKindsAreValid(t *testing.T) {
	for _, kind := range Kinds() {
		if !kind.Valid() {
			t.Errorf("kind %s is listed but not valid", kind)
		}
	}
	for _, name := range []string{"finding", "folder", ""} {
		if Kind(name).Valid() {
			t.Errorf("kind %q is valid", name)
		}
	}
}
