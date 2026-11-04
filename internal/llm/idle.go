package llm

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

type IdleWatch struct {
	timeout time.Duration
	fired   atomic.Bool
}

func NewIdleWatch(timeout time.Duration) *IdleWatch {
	return &IdleWatch{timeout: timeout}
}

func (w *IdleWatch) Middleware(r *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if w.timeout <= 0 {
		return next(r)
	}
	ctx, cancel := context.WithCancel(r.Context())
	timer := time.AfterFunc(w.timeout, func() {
		w.fired.Store(true)
		cancel()
	})
	resp, err := next(r.WithContext(ctx))
	if err != nil || resp == nil {
		timer.Stop()
		cancel()
		return resp, err
	}
	resp.Body = &idleBody{body: resp.Body, timer: timer, timeout: w.timeout, cancel: cancel}
	return resp, nil
}

func (w *IdleWatch) Fired() bool { return w != nil && w.fired.Load() }

func (w *IdleWatch) Failure(callerCtx, callCtx context.Context, provider string) error {
	if !w.Fired() || callerCtx.Err() != nil || callCtx.Err() != nil {
		return nil
	}
	return IdleTimeoutError(provider)
}

type idleBody struct {
	body    io.ReadCloser
	timer   *time.Timer
	timeout time.Duration
	cancel  context.CancelFunc
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	switch {
	case err != nil:
		b.timer.Stop()
	case n > 0:
		b.timer.Reset(b.timeout)
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	err := b.body.Close()
	b.cancel()
	return err
}
