package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const MaxAppDataBytes = 2 << 20

type PanelResult struct {
	Body     json.RawMessage
	Oversize int
	Result   queryproxy.Result
}

func PanelData(ctx context.Context, runner *queryproxy.Runner, logger *slog.Logger, req queryproxy.Request, withBody bool) (PanelResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{runner: runner, logger: logger}
	entry, res, err := h.renderRun(ctx, req)
	if errors.Is(err, errTooLarge) {
		return PanelResult{}, fmt.Errorf("%w: the panel data is over the size a chart can be sent in", datasource.ErrLimitExceeded)
	}
	if err != nil {
		return PanelResult{}, err
	}
	if !withBody {
		return PanelResult{Result: res}, nil
	}
	body, err := json.Marshal(dataBody(entry, false, entry.executedAt, ""))
	if err != nil {
		return PanelResult{}, fmt.Errorf("encode panel data: %w", err)
	}
	if len(body) > MaxAppDataBytes {
		return PanelResult{Oversize: len(body), Result: res}, nil
	}
	return PanelResult{Body: body, Result: res}, nil
}

type PanelWindow struct {
	Range         datasource.TimeRange
	MaxDataPoints int
}

func ResolvePanelWindow(d *spec.Dashboard, panel *spec.Panel, from, to string, maxDataPoints int, now time.Time) (PanelWindow, error) {
	al, err := params{from: from, to: to, maxDataPoints: maxDataPoints}.panelWindow(d, panel, now)
	if err != nil {
		return PanelWindow{}, err
	}
	return PanelWindow{Range: al.rng, MaxDataPoints: al.maxDataPoints}, nil
}
