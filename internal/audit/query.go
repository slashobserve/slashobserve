package audit

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ActionQueryRun Action = "query.run"

	MaxQueryBytes = 4096
	truncatedMark = " [truncated]"
)

type Query struct {
	Datasource string
	Text       string
	From, To   time.Time
	Rows       int
}

func NoteQuery(ctx context.Context, q Query) {
	st := stateOf(ctx)
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.queries = append(st.queries, q)
}

func Noted(ctx context.Context) []Query {
	st := stateOf(ctx)
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return slices.Clone(st.queries)
}

func RecordedQuery(text string) string {
	truncated := false
	if len(text) > MaxQueryBytes+maskSlackBytes {
		text, truncated = text[:runeStartAtOrBefore(text, MaxQueryBytes+maskSlackBytes)], true
	}
	text = maskSecrets(text)
	if len(text) <= MaxQueryBytes && !truncated {
		return text
	}
	return text[:runeStartAtOrBefore(text, MaxQueryBytes)] + truncatedMark
}

func runeStartAtOrBefore(text string, at int) int {
	at = min(at, len(text))
	for at > 0 && at < len(text) && !utf8.RuneStart(text[at]) {
		at--
	}
	return at
}

type queryRead struct {
	datasource string
	texts      []string
	from, to   time.Time
	rows       int
}

func groupQueries(queries []Query) []queryRead {
	var reads []queryRead
	for _, q := range queries {
		i := slices.IndexFunc(reads, func(r queryRead) bool { return r.datasource == q.Datasource })
		if i < 0 {
			reads = append(reads, queryRead{datasource: q.Datasource})
			i = len(reads) - 1
		}
		r := &reads[i]
		if !slices.Contains(r.texts, q.Text) {
			r.texts = append(r.texts, q.Text)
		}
		r.rows += q.Rows
		if !q.From.IsZero() && (r.from.IsZero() || q.From.Before(r.from)) {
			r.from = q.From
		}
		if q.To.After(r.to) {
			r.to = q.To
		}
	}
	return reads
}

func (r queryRead) text() string {
	return strings.Join(r.texts, "\n")
}

func (r queryRead) subject() string {
	span := max(r.to.Sub(r.from).Round(time.Minute), time.Minute)
	return strings.Join([]string{r.datasource, r.text(), span.String(), r.from.Truncate(span).UTC().Format(time.RFC3339)}, "\x00")
}

func (r queryRead) event(base Event) Event {
	data := maps.Clone(base.Data)
	if data == nil {
		data = map[string]any{}
	}
	data["query"] = RecordedQuery(r.text())
	data["row_count"] = r.rows
	if !r.from.IsZero() {
		data["from"] = r.from.UTC().Format(time.RFC3339)
	}
	if !r.to.IsZero() {
		data["to"] = r.to.UTC().Format(time.RFC3339)
	}
	base.Data = data
	base.Datasource = r.datasource
	base.Target = Target{Kind: "datasource", ID: r.datasource}
	return base
}

func (r *Recorder) queryEvents(ctx context.Context, st *state, base Event, fold bool) []Event {
	reads := groupQueries(st.takeQueries())
	events := make([]Event, 0, len(reads))
	for _, read := range reads {
		if fold && base.Outcome == OutcomeSuccess && r.queryRepeats.SeenRecently(ctx, read.subject(), time.Now()) {
			continue
		}
		events = append(events, read.event(base))
	}
	return events
}

func RecordNotedQueries(ctx context.Context, base Event) error {
	return current().recordNotedQueries(ctx, base)
}

func (r *Recorder) recordNotedQueries(ctx context.Context, base Event) error {
	st := stateOf(ctx)
	if st == nil || !st.hasQueries() {
		return nil
	}
	if r.db == nil {
		return ErrNotInstalled
	}
	base.Action = ActionQueryRun
	for _, e := range r.queryEvents(ctx, st, base, true) {
		if err := r.RecordNow(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
