package audittest

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func Middleware(t testing.TB, d *dbtest.Database) (middleware gin.HandlerFunc, flush func()) {
	t.Helper()
	recorder := audit.New(audit.Config{DB: d.DB})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	flushed := false
	flush = func() {
		if !flushed {
			flushed = true
			stop()
			<-done
		}
	}
	t.Cleanup(flush)
	return mw, flush
}
