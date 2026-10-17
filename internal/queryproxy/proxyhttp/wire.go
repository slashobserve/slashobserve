package proxyhttp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type wireFrame struct {
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Fields []wireField       `json:"fields"`
}

type wireField struct {
	Name   string               `json:"name"`
	Type   datasource.FieldType `json:"type"`
	Values []any                `json:"values"`
}

func marshalFrames(frames []datasource.Frame) ([]byte, error) {
	out := make([]wireFrame, len(frames))
	for i, f := range frames {
		wf := wireFrame{Name: f.Name, Labels: f.Labels, Fields: make([]wireField, len(f.Fields))}
		for j, field := range f.Fields {
			values, err := wireValues(field)
			if err != nil {
				return nil, err
			}
			wf.Fields[j] = wireField{Name: field.Name, Type: field.Type, Values: values}
		}
		out[i] = wf
	}
	return json.Marshal(out)
}

func wireValues(field datasource.Field) ([]any, error) {
	values := make([]any, len(field.Values))
	for i, v := range field.Values {
		switch x := v.(type) {
		case time.Time:
			if field.Type != datasource.FieldTime {
				return nil, fmt.Errorf("%s field %q holds a time value", field.Type, field.Name)
			}
			values[i] = x.UnixMilli()
		case float64:
			values[i] = datasource.Number(x)
		case float32:
			values[i] = datasource.Number(float64(x))
		case nil:
			values[i] = nil
		default:
			if field.Type == datasource.FieldTime {
				return nil, fmt.Errorf("time field %q holds a %T value", field.Name, v)
			}
			values[i] = v
		}
	}
	return values, nil
}
