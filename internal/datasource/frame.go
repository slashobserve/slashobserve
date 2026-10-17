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

func Points(frames []Frame) int {
	total := 0
	for _, f := range frames {
		total += f.Rows() * max(len(f.Fields)-1, 1)
	}
	return total
}
