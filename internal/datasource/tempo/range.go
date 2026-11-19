package tempo

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const futureRange = "the time range starts in the future, so no trace can match; pick a range that has already begun"

func rangeStartsInFuture(r datasource.TimeRange) error {
	if r.From.After(time.Now()) {
		return datasource.Errorf(datasource.ErrRangeInFuture, "%s", futureRange)
	}
	return nil
}
