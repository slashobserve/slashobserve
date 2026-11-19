package revisions

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxTimeZoneBytes = 64

type timeZoneKey struct{}

func CheckTimeZone(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "UTC" {
		return "", nil
	}
	if len(name) > maxTimeZoneBytes {
		return "", fmt.Errorf("time_zone must be at most %d bytes", maxTimeZoneBytes)
	}
	if _, err := spec.LoadZone(name); err != nil {
		return "", fmt.Errorf("time_zone: %w", err)
	}
	return name, nil
}

func WithTimeZone(ctx context.Context, zone string) context.Context {
	return context.WithValue(ctx, timeZoneKey{}, zone)
}

func timeZoneColumn(ctx context.Context) pgtype.Text {
	zone, _ := ctx.Value(timeZoneKey{}).(string)
	return pgtype.Text{String: zone, Valid: zone != ""}
}
