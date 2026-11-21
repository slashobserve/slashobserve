package filter

import (
	"reflect"
	"testing"
	"time"
)

func TestInputsCompileToParameterisedWhereClauses(t *testing.T) {
	const runID = "5b1e0a52-0b7c-4f08-9b1f-6a1d3d6a9a10"
	for _, tc := range []struct {
		text  string
		where string
		args  []any
	}{
		{"", "", []any{}},
		{"action:dashboard.update", "COALESCE((action = $3), false)", []any{"dashboard.update"}},
		{`action:"dashboard.*"`, "COALESCE((action = $3), false)", []any{"dashboard.*"}},
		{"action:dashboard.*", `COALESCE(action ILIKE $3 ESCAPE '\', false)`, []any{"dashboard.%"}},
		{"actor_name:*ish*", `COALESCE(actor_name ILIKE $3 ESCAPE '\', false)`, []any{"%ish%"}},
		{"not outcome:denied", "(NOT COALESCE((outcome = $3), false))", []any{"denied"}},
		{"outcome:denied or outcome:failed", "((COALESCE((outcome = $3), false) OR COALESCE((outcome = $4), false)))", []any{"denied", "failed"}},
		{"outcome:(denied or failed)", "((COALESCE((outcome = $3), false) OR COALESCE((outcome = $4), false)))", []any{"denied", "failed"}},
		{"outcome:denied and (status:403 or status:401)", "(COALESCE((outcome = $3), false) AND ((COALESCE((status = $4), false) OR COALESCE((status = $5), false))))", []any{"denied", int32(403), int32(401)}},
		{`actor_name:"Ishan Dubey"`, "COALESCE((actor_name = $3), false)", []any{"Ishan Dubey"}},
		{"status > 400", "COALESCE((status > $3), false)", []any{int32(400)}},
		{"duration_ms <= 12", "COALESCE((duration_ms <= $3), false)", []any{int32(12)}},
		{"client_ip:10.4.7.9/16", "COALESCE((client_ip <<= $3::inet), false)", []any{"10.4.0.0/16"}},
		{"client_ip:10.0.0.1", "COALESCE((client_ip = $3::inet), false)", []any{"10.0.0.1"}},
		{`client_ip:"::1"`, "COALESCE((client_ip = $3::inet), false)", []any{"::1"}},
		{"run_id:" + runID, "COALESCE((run_id = $3::uuid), false)", []any{runID}},
		{"target:dashboard", "COALESCE((target_kind = $3), false)", []any{"dashboard"}},
		{"target:dashboard/1 or target:folder/2", "((COALESCE((target_kind = $3 AND target_id = $4), false) OR COALESCE((target_kind = $5 AND target_id = $6), false)))", []any{"dashboard", "1", "folder", "2"}},
		{"target:dashboard/8f*", `COALESCE(concat(target_kind, chr(47) || target_id) ILIKE $3 ESCAPE '\', false)`, []any{"dashboard/8f%"}},
		{`occurred_at >= "2026-11-01T10:00:00Z"`, "COALESCE((occurred_at >= $3), false)", []any{time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)}},
		{"occurred_at < 2026-11-01", "COALESCE((occurred_at < $3), false)", []any{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}},
		{"occurred_at:2026-11-01", "COALESCE((occurred_at >= $3 AND occurred_at < $4), false)", []any{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}},
		{"occurred_at >= 2026-11-01", "COALESCE((occurred_at >= $3), false)", []any{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}},
		{"occurred_at > 2026-11-01", "COALESCE((occurred_at >= $3), false)", []any{time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}},
		{"occurred_at <= 2026-11-01", "COALESCE((occurred_at < $3), false)", []any{time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}},
		{`{"query":{"term":{"occurred_at":"2026-11-01"}}}`, "COALESCE((occurred_at >= $3 AND occurred_at < $4), false)", []any{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}},
		{`{"query":{"range":{"occurred_at":{"gte":"2026-11-01","lte":"2026-11-02"}}}}`, "COALESCE((occurred_at >= $3 AND occurred_at < $4), false)", []any{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)}},
		{`{"query":{"term":{"occurred_at":"2026-11-01T10:00:00Z"}}}`, "COALESCE((occurred_at = $3), false)", []any{time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)}},
		{"reason:*", "(reason IS NOT NULL)", []any{}},
		{"not reason:*", "(NOT (reason IS NOT NULL))", []any{}},
		{"act*:x", "(COALESCE((action = $3), false) OR COALESCE((actor_name = $4), false) OR COALESCE((actor_id IN (SELECT id FROM users WHERE lower(email) = lower($5))), false) OR COALESCE((actor_role = $6), false))", []any{"x", "x", "x", "x"}},
		{"checkout", "(search @@ plainto_tsquery('simple', $3))", []any{"checkout"}},
		{`"dashboard update"`, "(search @@ phraseto_tsquery('simple', $3))", []any{"dashboard update"}},
		{"check*", "(search @@ to_tsquery('simple', $3))", []any{"'check':*"}},
		{"check* failed", "((search @@ to_tsquery('simple', $3)) AND (search @@ plainto_tsquery('simple', $4)))", []any{"'check':*", "failed"}},
		{"it's*", "(search @@ to_tsquery('simple', $3))", []any{`'it''s':*`}},
		{`a\\b*`, "(search @@ to_tsquery('simple', $3))", []any{`'a\\b':*`}},
		{"not timeout", "(NOT (search @@ plainto_tsquery('simple', $3)))", []any{"timeout"}},

		{`{"query":{"term":{"outcome":"denied"}}}`, "COALESCE((outcome = $3), false)", []any{"denied"}},
		{`{"query":{"term":{"outcome":{"value":"denied"}}}}`, "COALESCE((outcome = $3), false)", []any{"denied"}},
		{`{"query":{"terms":{"outcome":["denied","failed"]}}}`, "(COALESCE((outcome = $3), false) OR COALESCE((outcome = $4), false))", []any{"denied", "failed"}},
		{`{"query":{"match_phrase":{"actor_name":"Ishan Dubey"}}}`, "COALESCE((actor_name = $3), false)", []any{"Ishan Dubey"}},
		{`{"query":{"match":{"status":403}}}`, "COALESCE((status = $3), false)", []any{int32(403)}},
		{`{"query":{"range":{"status":{"gte":400,"lt":500}}}}`, "COALESCE((status >= $3 AND status < $4), false)", []any{int32(400), int32(500)}},
		{`{"query":{"range":{"actor_name":{"gt":"a"}}}}`, "COALESCE((actor_name > $3), false)", []any{"a"}},
		{`{"query":{"wildcard":{"action":"dash*"}}}`, `COALESCE(action LIKE $3 ESCAPE '\', false)`, []any{"dash%"}},
		{`{"query":{"wildcard":{"action":{"value":"dash*","case_insensitive":true}}}}`, `COALESCE(action ILIKE $3 ESCAPE '\', false)`, []any{"dash%"}},
		{`{"query":{"wildcard":{"action":"a_b%?"}}}`, `COALESCE(action LIKE $3 ESCAPE '\', false)`, []any{`a\_b\%_`}},
		{`{"query":{"exists":{"field":"reason"}}}`, "(reason IS NOT NULL)", []any{}},
		{`{"query":{"match_all":{}}}`, "true", []any{}},
		{`{"query":{"match_none":{}}}`, "false", []any{}},
		{`{"query":{}}`, "", []any{}},
		{`{"query":{"bool":{"filter":{"term":{"outcome":"denied"}}}}}`, "(COALESCE((outcome = $3), false))", []any{"denied"}},
		{`{"query":{"bool":{"must":[{"term":{"outcome":"denied"}}],"should":[{"term":{"origin":"mcp"}}]}}}`, "(COALESCE((outcome = $3), false))", []any{"denied"}},
		{`{"query":{"bool":{"must_not":[{"term":{"outcome":"denied"}}]}}}`, "(NOT COALESCE((outcome = $3), false))", []any{"denied"}},
		{`{"query":{"bool":{"should":[{"term":{"outcome":"denied"}},{"term":{"outcome":"failed"}},{"term":{"origin":"mcp"}}],"minimum_should_match":2}}}`, "(((COALESCE((outcome = $3), false))::int + (COALESCE((outcome = $4), false))::int + (COALESCE((origin = $5), false))::int >= 2))", []any{"denied", "failed", "mcp"}},
		{`{"query":{"multi_match":{"query":"checkout","fields":["actor_name","target_name"],"type":"phrase"}}}`, "(COALESCE((actor_name = $3), false) OR COALESCE((target_name = $4), false))", []any{"checkout", "checkout"}},
		{`{"query":{"multi_match":{"query":"checkout"}}}`, "(search @@ plainto_tsquery('simple', $3))", []any{"checkout"}},
		{`{"query":{"query_string":{"query":"che*"}}}`, "(search @@ to_tsquery('simple', $3))", []any{"'che':*"}},
		{`{"query":{"term":{"client_ip":"10.0.0.0/8"}}}`, "COALESCE((client_ip <<= $3::inet), false)", []any{"10.0.0.0/8"}},
		{`{"aggs":{"a":{"max":{"field":"duration_ms"}}},"size":0}`, "", []any{}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			where, args := whereAndArgs(t, tc.text)
			if where != tc.where {
				t.Errorf("where\n got %s\nwant %s", where, tc.where)
			}
			if !reflect.DeepEqual(args, tc.args) {
				t.Errorf("args\n got %#v\nwant %#v", args, tc.args)
			}
		})
	}
}

func TestKnownResolvesAPatternAgainstFieldNamesOnly(t *testing.T) {
	for name, want := range map[string]bool{
		"actor_name": true,
		"actor.name": true,
		"actor":      true,
		"actor_*":    true,
		"*_name":     true,
		"actor.*":    false,
		"target.*":   false,
		"target":     true,
		"message":    false,
		"nosuch*":    false,
	} {
		if got := Known(name); got != want {
			t.Errorf("Known(%q) = %v, want %v", name, got, want)
		}
	}
}
