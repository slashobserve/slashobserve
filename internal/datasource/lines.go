package datasource

import (
	"context"
	"strconv"
	"time"
)

const TimeNsFieldName = "TimeNs"

type LineDirection string

const (
	Backward LineDirection = "backward"
	Forward  LineDirection = "forward"
)

type LinesRequest struct {
	Query     Query
	Range     TimeRange
	Direction LineDirection
	Limit     int
}

type LineSource interface {
	Lines(ctx context.Context, req LinesRequest) ([]Frame, error)
}

func TimeNsField(times []any) Field {
	values := make([]any, len(times))
	for i, v := range times {
		if t, ok := v.(time.Time); ok {
			values[i] = strconv.FormatInt(t.UnixNano(), 10)
		}
	}
	return Field{Name: TimeNsFieldName, Type: FieldString, Values: values}
}

func TimeNsOf(f Frame, row int) (int64, bool) {
	for _, field := range f.Fields {
		if field.Name != TimeNsFieldName {
			continue
		}
		if row < 0 || row >= len(field.Values) {
			return 0, false
		}
		s, ok := field.Values[row].(string)
		if !ok {
			return 0, false
		}
		ns, err := strconv.ParseInt(s, 10, 64)
		return ns, err == nil
	}
	return 0, false
}

type LagSource interface {
	TailLag() time.Duration
}
