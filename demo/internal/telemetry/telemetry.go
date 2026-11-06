package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const flushTimeout = 5 * time.Second

func Setup(ctx context.Context, service, pod string) (*slog.Logger, func(), error) {
	stdout := slog.NewJSONHandler(os.Stdout, nil).WithAttrs([]slog.Attr{
		slog.String("service", service),
		slog.String("pod", pod),
	})
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return slog.New(stdout), func() {}, nil
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", service),
		attribute.String("service.instance.id", pod),
		attribute.String("k8s.pod.name", pod),
		attribute.String("deployment.environment.name", "demo"),
	))
	if err != nil {
		return nil, nil, err
	}
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger := slog.New(slog.NewMultiHandler(
		stdout,
		otelslog.NewHandler("shop", otelslog.WithLoggerProvider(loggerProvider)),
	))
	shutdown := func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		err := errors.Join(tracerProvider.Shutdown(flushCtx), loggerProvider.Shutdown(flushCtx))
		if err != nil {
			slog.New(stdout).Error("telemetry flush failed", "error", err)
		}
	}
	return logger, shutdown, nil
}
