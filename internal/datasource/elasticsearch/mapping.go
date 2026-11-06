package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	mappingTTL       = time.Minute
	mappingWindow    = "now-7d"
	maxMappingFields = 5000
)

type FieldKind string

const (
	KindKeyword   FieldKind = "keyword"
	KindText      FieldKind = "text"
	KindNumber    FieldKind = "number"
	KindDate      FieldKind = "date"
	KindDateNanos FieldKind = "date_nanos"
	KindBoolean   FieldKind = "boolean"
	KindIP        FieldKind = "ip"
	KindNested    FieldKind = "nested"
	KindFlattened FieldKind = "flattened"
	KindObject    FieldKind = "object"
	KindOther     FieldKind = "other"
)

var kindOfType = map[string]FieldKind{
	"keyword":          KindKeyword,
	"constant_keyword": KindKeyword,
	"wildcard":         KindKeyword,
	"text":             KindText,
	"match_only_text":  KindText,
	"long":             KindNumber,
	"integer":          KindNumber,
	"short":            KindNumber,
	"byte":             KindNumber,
	"double":           KindNumber,
	"float":            KindNumber,
	"half_float":       KindNumber,
	"scaled_float":     KindNumber,
	"unsigned_long":    KindNumber,
	"date":             KindDate,
	"date_nanos":       KindDateNanos,
	"boolean":          KindBoolean,
	"ip":               KindIP,
	"nested":           KindNested,
	"flattened":        KindFlattened,
	"flat_object":      KindFlattened,
	"object":           KindObject,
}

type Field struct {
	Name         string
	Kind         FieldKind
	Aggregatable bool
	Searchable   bool
	Keyword      string
	Conflict     bool
}

type Mapping struct {
	Fields    map[string]Field
	Truncated bool
}

func (m *Mapping) Names() []string {
	return slices.Sorted(maps.Keys(m.Fields))
}

func (m *Mapping) IsDate(name string) bool {
	f, ok := m.Fields[name]
	return ok && (f.Kind == KindDate || f.Kind == KindDateNanos)
}

func (m *Mapping) termsField(name string) (string, error) {
	f, ok := m.Fields[name]
	switch {
	case !ok:
		return "", datasource.Errorf(datasource.ErrInvalidQuery, "field %s is not in the mapping", truncateText(name))
	case f.Aggregatable:
		return name, nil
	case f.Keyword != "":
		return f.Keyword, nil
	case f.Kind == KindText:
		return "", datasource.Errorf(datasource.ErrInvalidQuery, "field %s is text and has no keyword subfield; values cannot be listed", truncateText(name))
	}
	return "", datasource.Errorf(datasource.ErrInvalidQuery, "field %s is not aggregatable", truncateText(name))
}

var roleCandidates = struct{ message, level, service, traceID, spanID []string }{
	message: []string{"message", "body", "body.text", "log", "msg", "event.original"},
	level:   []string{"log.level", "severity.text", "severity_text", "level", "severity", "loglevel"},
	service: []string{"service.name", "resource.service.name", "resource.attributes.service.name", "resource.attributes.service_name", "service_name", "app"},
	traceID: []string{"trace.id", "traceId", "trace_id", "traceID"},
	spanID:  []string{"span.id", "spanId", "span_id", "spanID"},
}

func (m *Mapping) roles(overrides FieldRoles) (FieldRoles, error) {
	if !m.IsDate(overrides.Time) {
		return FieldRoles{}, datasource.Errorf(datasource.ErrNoTimeField, "time field %s is missing or is not a date", truncateText(overrides.Time))
	}
	out := FieldRoles{Time: overrides.Time}
	for _, role := range []struct {
		dst        *string
		override   string
		candidates []string
	}{
		{&out.Message, overrides.Message, roleCandidates.message},
		{&out.Level, overrides.Level, roleCandidates.level},
		{&out.Service, overrides.Service, roleCandidates.service},
		{&out.TraceID, overrides.TraceID, roleCandidates.traceID},
		{&out.SpanID, overrides.SpanID, roleCandidates.spanID},
	} {
		if role.override != "" {
			if _, ok := m.Fields[role.override]; !ok {
				return FieldRoles{}, datasource.Errorf(datasource.ErrNoRoleField, "field %s is not in the mapping", truncateText(role.override))
			}
			*role.dst = role.override
			continue
		}
		for _, candidate := range role.candidates {
			if _, ok := m.Fields[candidate]; ok {
				*role.dst = candidate
				break
			}
		}
	}
	return out, nil
}

type mappingCache struct {
	mu     sync.Mutex
	value  *Mapping
	at     time.Time
	flight singleflight.Group
}

// The shared fetch runs detached from the first caller's context so one cancelled request cannot fail the others.
func (d *Datasource) Mapping(ctx context.Context) (*Mapping, error) {
	d.mapping.mu.Lock()
	value, at := d.mapping.value, d.mapping.at
	d.mapping.mu.Unlock()
	if value != nil && time.Since(at) < mappingTTL {
		return value, nil
	}
	results := d.mapping.flight.DoChan("mapping", func() (any, error) {
		fctx, cancel := bound(context.WithoutCancel(ctx), d.opts.Limits.Timeout)
		defer cancel()
		m, err := d.fetchMapping(fctx)
		if err != nil {
			return nil, err
		}
		d.mapping.mu.Lock()
		d.mapping.value, d.mapping.at = m, time.Now()
		d.mapping.mu.Unlock()
		return m, nil
	})
	select {
	case r := <-results:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*Mapping), nil
	case <-ctx.Done():
		return nil, transportError(ctx, ctx.Err())
	}
}

func (d *Datasource) Roles(ctx context.Context) (FieldRoles, error) {
	m, err := d.Mapping(ctx)
	if err != nil {
		return FieldRoles{}, err
	}
	return m.roles(d.opts.Fields)
}

func (d *Datasource) fetchMapping(ctx context.Context) (*Mapping, error) {
	recentIndices := map[string]any{"index_filter": map[string]any{"range": map[string]any{d.opts.TimeField: map[string]any{"gte": mappingWindow}}}}
	m, err := d.fieldCaps(ctx, recentIndices)
	if err != nil || len(m.Fields) > 0 {
		return m, err
	}
	var everyIndex any
	return d.fieldCaps(ctx, everyIndex)
}

func (d *Datasource) fieldCaps(ctx context.Context, body any) (*Mapping, error) {
	params := url.Values{"fields": {"*"}}
	maps.Copy(params, lenientIndices)
	raw, err := d.call(ctx, http.MethodPost, d.indexPath("/_field_caps"), params, body)
	if err != nil {
		return nil, err
	}
	return parseFieldCaps(raw)
}

type fieldCapability struct {
	Type          string `json:"type"`
	MetadataField bool   `json:"metadata_field"`
	Searchable    bool   `json:"searchable"`
	Aggregatable  bool   `json:"aggregatable"`
}

func parseFieldCaps(raw []byte) (*Mapping, error) {
	var resp struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Fields == nil {
		return nil, unreadable()
	}
	m := &Mapping{Fields: make(map[string]Field, len(resp.Fields))}
	for name, caps := range resp.Fields {
		f, ok, err := foldField(name, caps)
		if err != nil {
			return nil, err
		}
		if ok {
			m.Fields[name] = f
		}
	}
	for name, f := range m.Fields {
		if f.Kind != KindText {
			continue
		}
		if sub, ok := m.Fields[name+".keyword"]; ok && sub.Kind == KindKeyword && sub.Aggregatable {
			f.Keyword = sub.Name
			m.Fields[name] = f
		}
	}
	if len(m.Fields) > maxMappingFields {
		m.Truncated = true
		for _, name := range m.Names()[maxMappingFields:] {
			delete(m.Fields, name)
		}
	}
	return m, nil
}

// Capabilities are read in document order because a conflicting field keeps the first type the server listed.
func foldField(name string, caps json.RawMessage) (Field, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(caps))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return Field{}, false, unreadable()
	}
	var field Field
	types := 0
	for dec.More() {
		if _, err := dec.Token(); err != nil {
			return Field{}, false, unreadable()
		}
		var capability fieldCapability
		if err := dec.Decode(&capability); err != nil {
			return Field{}, false, unreadable()
		}
		if capability.MetadataField || strings.HasPrefix(name, "_") {
			return Field{}, false, nil
		}
		types++
		if types > 1 {
			continue
		}
		kind, known := kindOfType[capability.Type]
		if !known {
			kind = KindOther
		}
		field = Field{Name: name, Kind: kind, Aggregatable: capability.Aggregatable, Searchable: capability.Searchable}
	}
	if types == 0 {
		return Field{}, false, nil
	}
	field.Conflict = types > 1
	return field, true, nil
}
