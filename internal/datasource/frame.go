package datasource

import "math"

type FieldType string

const (
	FieldTime   FieldType = "time"
	FieldNumber FieldType = "number"
	FieldString FieldType = "string"
	FieldBool   FieldType = "bool"
	FieldOther  FieldType = "other"
)

type Field struct {
	Name   string    `json:"name"`
	Type   FieldType `json:"type"`
	Values []any     `json:"values"`
}

type Frame struct {
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Fields []Field           `json:"fields"`
	Meta   *FrameMeta        `json:"meta,omitempty"`
}

type FrameMeta struct {
	Truncated      bool  `json:"truncated,omitempty"`
	Partial        bool  `json:"partial,omitempty"`
	RowsTotal      int   `json:"rows_total,omitempty"`
	JobsDone       int64 `json:"jobs_done,omitempty"`
	JobsTotal      int64 `json:"jobs_total,omitempty"`
	InspectedBytes int64 `json:"inspected_bytes,omitempty"`
	Bytes          int64 `json:"bytes,omitempty"`
}

func MergeMeta(frames []Frame) FrameMeta {
	var merged FrameMeta
	for _, f := range frames {
		if f.Meta == nil {
			continue
		}
		merged.Truncated = merged.Truncated || f.Meta.Truncated
		merged.Partial = merged.Partial || f.Meta.Partial
		if merged.RowsTotal == 0 {
			merged.RowsTotal = f.Meta.RowsTotal
		}
		merged.JobsDone += f.Meta.JobsDone
		merged.JobsTotal += f.Meta.JobsTotal
		merged.InspectedBytes += f.Meta.InspectedBytes
		merged.Bytes += f.Meta.Bytes
	}
	return merged
}

// NaN and Inf are not representable in JSON; they render as gaps.
func Number(v float64) any {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}

func (f Frame) Rows() int {
	if len(f.Fields) == 0 {
		return 0
	}
	return len(f.Fields[0].Values)
}

func (f Frame) IsSeries() bool {
	if len(f.Fields) < 2 || f.Fields[0].Type != FieldTime {
		return false
	}
	for _, field := range f.Fields[1:] {
		if field.Type != FieldNumber {
			return false
		}
	}
	return true
}

func Points(frames []Frame) int {
	total := 0
	for _, f := range frames {
		total += f.Rows() * max(len(f.Fields)-1, 1)
	}
	return total
}
