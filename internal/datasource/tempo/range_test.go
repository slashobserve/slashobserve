package tempo

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestRangeStartingInTheFutureHasItsOwnKind(t *testing.T) {
	now := time.Now()
	err := rangeStartsInFuture(datasource.TimeRange{From: now.Add(time.Hour), To: now.Add(2 * time.Hour)})
	if !errors.Is(err, datasource.ErrRangeInFuture) || !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "starts in the future") {
		t.Fatalf("err = %v", err)
	}
	if err := rangeStartsInFuture(datasource.TimeRange{From: now.Add(-time.Hour), To: now.Add(time.Hour)}); err != nil {
		t.Fatalf("a range that has begun was refused: %v", err)
	}
}
