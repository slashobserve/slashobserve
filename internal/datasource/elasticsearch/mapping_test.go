package elasticsearch_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

func TestMappingFoldsFieldCaps(t *testing.T) {
	ds := serve(t, healthyServer(t))
	m, err := ds.Mapping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]elasticsearch.Field{
		"@timestamp":                {Name: "@timestamp", Kind: elasticsearch.KindDateNanos, Aggregatable: true, Searchable: true},
		"event.ingested":            {Name: "event.ingested", Kind: elasticsearch.KindDate, Aggregatable: true, Searchable: true},
		"message":                   {Name: "message", Kind: elasticsearch.KindText, Searchable: true, Keyword: "message.keyword"},
		"message.keyword":           {Name: "message.keyword", Kind: elasticsearch.KindKeyword, Aggregatable: true, Searchable: true},
		"log.level":                 {Name: "log.level", Kind: elasticsearch.KindKeyword, Aggregatable: true, Searchable: true},
		"labels":                    {Name: "labels", Kind: elasticsearch.KindFlattened, Aggregatable: true, Searchable: true},
		"error.stack_trace":         {Name: "error.stack_trace", Kind: elasticsearch.KindText, Searchable: true},
		"http.response.status_code": {Name: "http.response.status_code", Kind: elasticsearch.KindNumber, Aggregatable: true, Searchable: true},
		"host.ip":                   {Name: "host.ip", Kind: elasticsearch.KindIP, Aggregatable: true, Searchable: true},
	}
	for name, field := range want {
		if got := m.Fields[name]; got != field {
			t.Errorf("field %s = %+v, want %+v", name, got, field)
		}
	}
	if seq := m.Fields["event.sequence"]; !seq.Conflict || seq.Kind == "" {
		t.Errorf("event.sequence = %+v, want a conflict with the first type kept", seq)
	}
	for _, metadata := range []string{"_id", "_index", "_source"} {
		if _, ok := m.Fields[metadata]; ok {
			t.Errorf("metadata field %s is in the mapping", metadata)
		}
	}
	if m.Truncated {
		t.Error("mapping reported truncated")
	}
}

func TestMappingKeepsTheFirstTypeOfAConflictAndTruncates(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"indices":["a"],"fields":{"_seq_no":{"long":{"type":"long","searchable":true,"aggregatable":true}},"seq":{"long":{"type":"long","metadata_field":false,"searchable":true,"aggregatable":true},"keyword":{"type":"keyword","metadata_field":false,"searchable":true,"aggregatable":true}},"ts":{"date":{"type":"date","metadata_field":false,"searchable":true,"aggregatable":true}}`)
	for i := range 5100 {
		fmt.Fprintf(&b, `,"zfill%04d":{"keyword":{"type":"keyword","metadata_field":false,"searchable":true,"aggregatable":true}}`, i)
	}
	b.WriteString(`}}`)
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_field_caps", 200, b.String())
	m, err := serve(t, f, func(o *elasticsearch.Options) { o.TimeField = "ts" }).Mapping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if seq := m.Fields["seq"]; seq.Kind != elasticsearch.KindNumber || !seq.Conflict {
		t.Fatalf("seq = %+v, want the first listed type with a conflict", seq)
	}
	if !m.Truncated || len(m.Fields) != 5000 {
		t.Fatalf("truncated = %v, fields = %d", m.Truncated, len(m.Fields))
	}
	if _, ok := m.Fields["zfill5099"]; ok {
		t.Fatal("the last field by name survived truncation")
	}
	if _, ok := m.Fields["_seq_no"]; ok {
		t.Error("an unflagged metadata field is in the mapping")
	}
}

func TestMappingFallsBackToEveryIndexWhenNoneIsRecent(t *testing.T) {
	f := healthyServer(t)
	caps := fieldCaps(t)
	f.set(http.MethodPost, "/logs-*/_field_caps", func(r request) reply {
		if strings.Contains(r.body, "index_filter") {
			return reply{body: `{"indices":[],"fields":{}}`}
		}
		return reply{body: caps}
	})
	m, err := serve(t, f).Mapping(context.Background())
	if err != nil || len(m.Fields) == 0 {
		t.Fatalf("Mapping = %v, %v", m, err)
	}
	if f.count("/logs-*/_field_caps") != 2 || f.last(t).body != "" {
		t.Fatalf("expected a second, unfiltered field caps call; got %d calls, last body %q", f.count("/logs-*/_field_caps"), f.last(t).body)
	}
}

func TestMappingIsCachedAndFetchedOnce(t *testing.T) {
	f := healthyServer(t)
	release := make(chan struct{})
	caps := fieldCaps(t)
	f.set(http.MethodPost, "/logs-*/_field_caps", func(request) reply {
		<-release
		return reply{body: caps}
	})
	ds := serve(t, f)
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ds.Mapping(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if _, err := ds.Mapping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/logs-*/_field_caps"); n != 1 {
		t.Fatalf("field caps fetched %d times, want once", n)
	}
}

func TestMappingWaiterHonoursItsOwnDeadline(t *testing.T) {
	f := healthyServer(t)
	release := make(chan struct{})
	f.set(http.MethodPost, "/logs-*/_field_caps", func(request) reply {
		<-release
		return reply{body: `{"fields":{}}`}
	})
	ds := serve(t, f)
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ds.Mapping(ctx)
	if !errors.Is(err, datasource.ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
}

func TestMappingUnreadable(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_field_caps", 200, `{"fields": 1}`)
	if _, err := serve(t, f).Mapping(context.Background()); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestRoles(t *testing.T) {
	ds := serve(t, healthyServer(t))
	roles, err := ds.Roles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := elasticsearch.FieldRoles{Time: "@timestamp", Message: "message", Level: "log.level", Service: "service.name", TraceID: "trace.id", SpanID: "span.id"}
	if roles != want {
		t.Fatalf("roles = %+v, want %+v", roles, want)
	}

	overridden := serve(t, healthyServer(t), func(o *elasticsearch.Options) {
		o.Fields = elasticsearch.FieldRoles{Message: "error.stack_trace", Service: "tags"}
	})
	roles, err = overridden.Roles(context.Background())
	if err != nil || roles.Message != "error.stack_trace" || roles.Service != "tags" || roles.Level != "log.level" {
		t.Fatalf("overridden roles = %+v, %v", roles, err)
	}

	absent := serve(t, healthyServer(t), func(o *elasticsearch.Options) { o.Fields.Level = "severity" })
	if _, err := absent.Roles(context.Background()); !errors.Is(err, datasource.ErrNoRoleField) {
		t.Fatalf("absent override error = %v, want ErrNoRoleField", err)
	}

	wrongTime := serve(t, healthyServer(t), func(o *elasticsearch.Options) { o.TimeField = "message" })
	if _, err := wrongTime.Roles(context.Background()); !errors.Is(err, datasource.ErrNoTimeField) {
		t.Fatalf("text time field error = %v, want ErrNoTimeField", err)
	}
}

func TestRolesFallThroughTheCandidateLists(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_field_caps", 200, `{"fields":{
		"@timestamp":{"date":{"type":"date","metadata_field":false,"searchable":true,"aggregatable":true}},
		"body":{"text":{"type":"text","metadata_field":false,"searchable":true,"aggregatable":false}},
		"severity_text":{"keyword":{"type":"keyword","metadata_field":false,"searchable":true,"aggregatable":true}},
		"resource.attributes.service.name":{"keyword":{"type":"keyword","metadata_field":false,"searchable":true,"aggregatable":true}},
		"traceId":{"keyword":{"type":"keyword","metadata_field":false,"searchable":true,"aggregatable":true}}}}`)
	roles, err := serve(t, f).Roles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := elasticsearch.FieldRoles{Time: "@timestamp", Message: "body", Level: "severity_text", Service: "resource.attributes.service.name", TraceID: "traceId"}
	if roles != want {
		t.Fatalf("roles = %+v, want %+v", roles, want)
	}
}
