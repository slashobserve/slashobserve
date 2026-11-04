package llm

import (
	"context"
	"net/http"
)

type CallTrace struct {
	Attempts   int
	Status     int
	RetryAfter string
}

type callTraceKey struct{}

func WithCallTrace(ctx context.Context) (context.Context, *CallTrace) {
	trace := &CallTrace{}
	return context.WithValue(ctx, callTraceKey{}, trace), trace
}

func TraceAttempt(r *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	trace, _ := r.Context().Value(callTraceKey{}).(*CallTrace)
	if trace == nil {
		return next(r)
	}
	trace.Attempts++
	resp, err := next(r)
	trace.Status = 0
	trace.RetryAfter = ""
	if resp != nil {
		trace.Status = resp.StatusCode
		if v := resp.Header.Get("Retry-After"); IsPlainToken(v) {
			trace.RetryAfter = v
		}
	}
	return resp, err
}
