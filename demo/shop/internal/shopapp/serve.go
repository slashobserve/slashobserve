package shopapp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/slashobserve/slashobserve/demo/shop/internal/telemetry"
)

const shutdownTimeout = 5 * time.Second

func Serve(ctx context.Context, lookup func(string) (string, bool)) error {
	cfg, err := ConfigFromEnv(lookup)
	if err != nil {
		return err
	}
	logger, flush, err := telemetry.Setup(ctx, cfg.Service, cfg.Pod)
	if err != nil {
		return fmt.Errorf("set up telemetry: %w", err)
	}
	defer flush()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newApp(cfg, logger),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	logger.Info("shop listening", "addr", listener.Addr().String())

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
