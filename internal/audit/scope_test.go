package audit

import (
	"maps"
	"testing"
	"time"
)

func TestARequestIsARepeatOnlyWhenEveryReadInItWas(t *testing.T) {
	for name, tc := range map[string]struct {
		repeats []bool
		want    bool
	}{
		"no read":                 {nil, false},
		"one repeated read":       {[]bool{true}, true},
		"two repeated reads":      {[]bool{true, true}, true},
		"a repeat and a new read": {[]bool{true, false}, false},
		"a new read and a repeat": {[]bool{false, true}, false},
		"one new read":            {[]bool{false}, false},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := Open(t.Context(), Scope{})
			for _, repeated := range tc.repeats {
				NoteRead(ctx, Read{Repeated: repeated})
			}
			if got := stateOf(ctx).repeatsARead("dashboard.view", OutcomeSuccess); got != tc.want {
				t.Fatalf("repeatsARead = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAQueryRunCarriesTheWidestWindowItsReadsCovered(t *testing.T) {
	ctx := Open(t.Context(), Scope{})
	st := stateOf(ctx)
	if got := st.withReadWindow("query.run", map[string]any{"stream": true}); len(got) != 1 {
		t.Fatalf("data without a read = %v, want it unchanged", got)
	}
	at := func(hour int) time.Time { return time.Date(2026, 11, 21, hour, 0, 0, 0, time.FixedZone("IST", 19800)) }
	NoteRead(ctx, Read{From: at(10), To: at(11)})
	NoteRead(ctx, Read{From: at(9), To: at(10)})
	given := map[string]any{"stream": true}
	got := st.withReadWindow("query.run", given)
	want := map[string]any{"stream": true, "from": "2026-11-21T03:30:00Z", "to": "2026-11-21T05:30:00Z"}
	if !maps.Equal(got, want) {
		t.Fatalf("data = %v, want %v", got, want)
	}
	if len(given) != 1 {
		t.Fatalf("the handler's own data was changed to %v", given)
	}
	if got := st.withReadWindow("dashboard.view", nil); got != nil {
		t.Fatalf("an action without from and to keys got %v", got)
	}
}

func TestARequestNamesTheDatasourceItsReadsWentThrough(t *testing.T) {
	ctx := Open(t.Context(), Scope{})
	st := stateOf(ctx)
	if got := st.readDatasource(); got != "" {
		t.Fatalf("datasource before any read = %q", got)
	}
	NoteRead(ctx, Read{Datasource: "slashobserve-audit"})
	NoteRead(ctx, Read{})
	if got := st.readDatasource(); got != "slashobserve-audit" {
		t.Fatalf("datasource = %q, want the one the read named", got)
	}
}
