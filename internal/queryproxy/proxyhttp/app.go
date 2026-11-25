package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const MaxAppDataBytes = 2 << 20

type PanelResult struct {
	Body   json.RawMessage
	Result queryproxy.Result
}

func PanelData(ctx context.Context, runner *queryproxy.Runner, logger *slog.Logger, req queryproxy.Request) (PanelResult, error) {
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
	body, err := json.Marshal(dataBody(entry, false, entry.executedAt, ""))
	if err != nil {
		return PanelResult{}, fmt.Errorf("encode panel data: %w", err)
	}
	if len(body) > MaxAppDataBytes {
		return PanelResult{}, fmt.Errorf("%w: the panel data is %d KiB, over the %d KiB a chart can be sent in; ask for a shorter range or fewer series", datasource.ErrLimitExceeded, len(body)>>10, MaxAppDataBytes>>10)
	}
	return PanelResult{Body: body, Result: res}, nil
}
