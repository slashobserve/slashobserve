package audit

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

type written struct {
	at     time.Time
	id     uuid.UUID
	action Action
}

type state struct {
	mu        sync.Mutex
	scope     Scope
	inTx      []written
	committed bool
	reads     int
	repeated  int
	readFrom  time.Time
	readTo    time.Time
	readOn    string
	result    *Result
}

type scopeKey struct{}

func Open(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, &state{scope: s})
}

func stateOf(ctx context.Context) *state {
	st, _ := ctx.Value(scopeKey{}).(*state)
	return st
}

func ScopeOf(ctx context.Context) (Scope, bool) {
	st := stateOf(ctx)
	if st == nil {
		return Scope{}, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.scope, true
}

func Update(ctx context.Context, fn func(*Scope)) {
	st := stateOf(ctx)
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	fn(&st.scope)
}

func Conclude(ctx context.Context, r Result) {
	st := stateOf(ctx)
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.result = &r
}

func Skip(ctx context.Context) {
	if st := stateOf(ctx); st != nil {
		st.markCommitted()
	}
}

type Read struct {
	Datasource string
	From, To   time.Time
	Repeated   bool
}

func NoteRead(ctx context.Context, r Read) {
	st := stateOf(ctx)
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.reads++
	if r.Repeated {
		st.repeated++
	}
	if st.readFrom.IsZero() || r.From.Before(st.readFrom) {
		st.readFrom = r.From
	}
	if r.To.After(st.readTo) {
		st.readTo = r.To
	}
	if r.Datasource != "" {
		st.readOn = r.Datasource
	}
}

func (st *state) readDatasource() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.readOn
}

func (st *state) withReadWindow(action Action, data map[string]any) map[string]any {
	st.mu.Lock()
	from, to := st.readFrom, st.readTo
	st.mu.Unlock()
	spec, _ := Lookup(action)
	if from.IsZero() || !slices.Contains(spec.Data, "from") {
		return data
	}
	out := maps.Clone(data)
	if out == nil {
		out = map[string]any{}
	}
	out["from"], out["to"] = from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)
	return out
}

func (st *state) wroteInTx(w written) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.inTx = append(st.inTx, w)
}

func (st *state) markCommitted() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.committed = true
}

func (st *state) unsettled() (bool, []written, *Result) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.committed, append([]written(nil), st.inTx...), st.result
}

func (st *state) repeatsARead(action Action, outcome Outcome) bool {
	st.mu.Lock()
	everyReadRepeated := st.reads > 0 && st.repeated == st.reads
	st.mu.Unlock()
	spec, known := Lookup(action)
	return everyReadRepeated && outcome == OutcomeSuccess && known && spec.Class == ClassAccess
}
