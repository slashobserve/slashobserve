package elasticsearch

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type hit struct {
	Index  string          `json:"_index"`
	ID     string          `json:"_id"`
	Source json.RawMessage `json:"_source"`
}

func parseHits(raw []byte) ([]hit, error) {
	var resp struct {
		Hits struct {
			Hits []hit `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, unreadable()
	}
	return resp.Hits.Hits, nil
}

var sourceTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

func hitsFrame(hits []hit, roles FieldRoles) datasource.Frame {
	type column struct {
		name string
		role string
	}
	columns := []column{{"Time", roles.Time}, {"TimeNs", ""}, {"Body", roles.Message}, {"level", roles.Level}, {"ID", ""}, {"Index", ""}, {"service", roles.Service}, {"trace_id", roles.TraceID}, {"span_id", roles.SpanID}, {"Fields", ""}}
	columns = slices.DeleteFunc(columns, func(c column) bool {
		switch c.name {
		case "level", "service", "trace_id", "span_id":
			return c.role == ""
		}
		return false
	})
	fields := make([]datasource.Field, len(columns))
	for i, c := range columns {
		typ := datasource.FieldString
		if c.name == "Time" {
			typ = datasource.FieldTime
		}
		fields[i] = datasource.Field{Name: c.name, Type: typ, Values: make([]any, 0, len(hits))}
	}
	for _, h := range hits {
		source := flattenSource(h.Source)
		at := sourceTime(takeValue(source, roles.Time))
		values := make([]any, len(columns))
		for i, c := range columns {
			switch c.name {
			case "Time":
				values[i] = at
			case "TimeNs":
				values[i] = strconv.FormatInt(at.UnixNano(), 10)
			case "Body":
				values[i] = stringValue(takeValue(source, roles.Message))
			case "ID":
				values[i] = h.ID
			case "Index":
				values[i] = h.Index
			default:
				if raw := takeValue(source, c.role); raw != nil {
					values[i] = stringValue(raw)
				}
			}
		}
		rest, _ := json.Marshal(source)
		values[len(values)-1] = string(rest)
		for i := range columns {
			fields[i].Values = append(fields[i].Values, values[i])
		}
	}
	return datasource.Frame{Fields: fields}
}

func flattenSource(raw json.RawMessage) map[string]any {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var source map[string]any
	if dec.Decode(&source) != nil {
		return map[string]any{}
	}
	flat := map[string]any{}
	flatten("", source, flat)
	return flat
}

func flatten(prefix string, source map[string]any, into map[string]any) {
	for key, value := range source {
		name := prefix + key
		if nested, ok := value.(map[string]any); ok && len(nested) > 0 {
			flatten(name+".", nested, into)
			continue
		}
		into[name] = value
	}
}

func takeValue(source map[string]any, name string) any {
	if name == "" {
		return nil
	}
	v, ok := source[name]
	if !ok {
		return nil
	}
	delete(source, name)
	return v
}

func stringValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func sourceTime(v any) time.Time {
	switch x := v.(type) {
	case string:
		for _, layout := range sourceTimeLayouts {
			if at, err := time.Parse(layout, x); err == nil {
				return at.UTC()
			}
		}
		if ms, err := strconv.ParseInt(x, 10, 64); err == nil {
			return time.UnixMilli(ms).UTC()
		}
	case json.Number:
		if ms, err := x.Int64(); err == nil {
			return time.UnixMilli(ms).UTC()
		}
		if f, err := x.Float64(); err == nil {
			return time.UnixMilli(int64(f)).UTC()
		}
	}
	return time.Time{}
}
