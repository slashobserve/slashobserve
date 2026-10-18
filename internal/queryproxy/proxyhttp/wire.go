package proxyhttp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type wireFrame struct {
	Ref    string            `json:"ref"`
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Fields []wireField       `json:"fields"`
}

type wireField struct {
	Name   string               `json:"name"`
	Type   datasource.FieldType `json:"type"`
	Values []any                `json:"values"`
}

func marshalFrames(queries []queryproxy.QueryResult) ([]byte, error) {
	out := []wireFrame{}
	for _, q := range queries {
		for _, f := range q.Frames {
			wf := wireFrame{Ref: q.Ref, Name: f.Name, Labels: f.Labels, Fields: make([]wireField, len(f.Fields))}
			for j, field := range f.Fields {
				values, err := wireValues(field)
				if err != nil {
					return nil, err
				}
				wf.Fields[j] = wireField{Name: field.Name, Type: field.Type, Values: values}
			}
			out = append(out, wf)
		}
	}
	return json.Marshal(out)
}

func joinFrames(parts [][]byte) []byte {
	out := []byte{'['}
	for _, part := range parts {
		if len(part) <= len("[]") {
			continue
		}
		inner := part[1 : len(part)-1]
		if len(out) > 1 {
			out = append(out, ',')
		}
		out = append(out, inner...)
	}
	return append(out, ']')
}

func wireValues(field datasource.Field) ([]any, error) {
	values := make([]any, len(field.Values))
	for i, v := range field.Values {
		if _, isTime := v.(time.Time); field.Type == datasource.FieldTime && v != nil && !isTime {
			return nil, fmt.Errorf("time field %q holds a %T value", field.Name, v)
		}
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
			values[i] = v
		}
	}
	return values, nil
}
