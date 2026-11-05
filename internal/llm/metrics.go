package llm

import (
	"context"
	"errors"
	"time"
)

type Metrics interface {
	ModelCall(provider, model, outcome string, took time.Duration, usage Usage)
}

func WithMetrics(p Provider, m Metrics) Provider {
	return meteredProvider{Provider: p, metrics: m}
}

type meteredProvider struct {
	Provider
	metrics Metrics
}

func (p meteredProvider) Start(ctx context.Context, req Request) (Session, error) {
	session, err := p.Provider.Start(ctx, req)
	if err != nil {
		return nil, err
	}
	return meteredSession{Session: session, provider: p.Name(), model: p.Model(), metrics: p.metrics}, nil
}

type meteredSession struct {
	Session
	provider string
	model    string
	metrics  Metrics
}

func (s meteredSession) Next(ctx context.Context, results []ToolResult) (Turn, error) {
	start := time.Now()
	turn, err := s.Session.Next(ctx, results)
	s.metrics.ModelCall(s.provider, s.model, callOutcome(err), time.Since(start), turn.Usage)
	return turn, err
}

func callOutcome(err error) string {
	var callErr *CallError
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &callErr):
		return string(callErr.Failure)
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return string(CallTimeout)
	}
	return "error"
}
