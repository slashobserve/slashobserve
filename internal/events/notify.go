package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var errEmptyNotice = errors.New("notification carries neither a sequence nor a live event")

type notice struct {
	Seq int64 `json:"seq,omitempty"`
	*Event
}

func Notify(ctx context.Context, q *dbsqlc.Queries, seq int64, live *Event) error {
	n := notice{Event: live}
	if seq > 0 {
		n.Seq = seq
	}
	if n.Seq == 0 && live == nil {
		return errEmptyNotice
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: Channel, Payload: string(payload)}); err != nil {
		return fmt.Errorf("notify event: %w", err)
	}
	return nil
}
