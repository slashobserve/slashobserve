package audit

import (
	"context"
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
	queries   []Query
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

func Concluded(ctx context.Context) (Result, bool) {
	st := stateOf(ctx)
	if st == nil {
		return Result{}, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.result == nil {
		return Result{}, false
	}
	return *st.result, true
}

func Skip(ctx context.Context) {
	if st := stateOf(ctx); st != nil {
		st.markCommitted()
	}
}

func (st *state) takeQueries() []Query {
	st.mu.Lock()
	defer st.mu.Unlock()
	queries := st.queries
	st.queries = nil
	return queries
}

func (st *state) hasQueries() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.queries) > 0
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
