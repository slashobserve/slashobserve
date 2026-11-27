package inboundmcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func yes() *bool { b := true; return &b }

func readOnly(name, description string) Found {
	return Found{Name: name, Description: description, InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`), ReadOnly: yes()}
}

func storedFrom(t *testing.T, state ToolState, f Found) Tool {
	t.Helper()
	l, _ := listingOf([]Found{f})
	if len(l) != 1 {
		t.Fatalf("listing = %d", len(l))
	}
	return Tool{Name: f.Name, State: state, Pinned: l[0].def, ApprovedBy: "admin", ApprovedAt: time.Unix(1, 0)}
}

func TestToolMachineIsConsistent(t *testing.T) {
	if err := ToolMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestSourceStateFollowsTheLastDiscovery(t *testing.T) {
	cases := []struct {
		checked bool
		failure string
		want    string
	}{
		{false, "", StateUnchecked},
		{false, "unauthorized", StateUnchecked},
		{true, "unauthorized", StateUnreachable},
		{true, "", StateReady},
	}
	for _, c := range cases {
		if got := sourceState(c.checked, c.failure); got != c.want {
			t.Errorf("sourceState(%v, %q) = %q, want %q", c.checked, c.failure, got, c.want)
		}
	}
}

func TestHashCoversNameDescriptionAndSchema(t *testing.T) {
	base := hashOf("a", "d", json.RawMessage(`{}`))
	for name, other := range map[string]string{
		"name":        hashOf("b", "d", json.RawMessage(`{}`)),
		"description": hashOf("a", "e", json.RawMessage(`{}`)),
		"schema":      hashOf("a", "d", json.RawMessage(`{"a":1}`)),
		"boundary":    hashOf("ad", "", json.RawMessage(`{}`)),
	} {
		if other == base {
			t.Errorf("changing the %s did not change the hash", name)
		}
	}
	if hashOf("a", "d", json.RawMessage(`{}`)) != base {
		t.Error("hash is not stable")
	}
}

func TestSchemaKeyOrderDoesNotChangeTheHash(t *testing.T) {
	a, err := canonicalSchema(json.RawMessage(`{"b":1,"a":{"y":2,"x":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalSchema(json.RawMessage(`{"a":{"x":3,"y":2},"b":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if hashOf("t", "d", a) != hashOf("t", "d", b) {
		t.Error("the same schema in another key order hashes differently")
	}
}

func TestEmptySchemaBecomesAnObject(t *testing.T) {
	for _, raw := range []string{"", "  ", "null"} {
		got, err := canonicalSchema(json.RawMessage(raw))
		if err != nil || string(got) != `{"type":"object"}` {
			t.Errorf("canonicalSchema(%q) = %s, %v", raw, got, err)
		}
	}
}

func TestToolNamesMustBeUsableAndUnique(t *testing.T) {
	for _, name := range []string{"search", "a.b-c_d", "Tool9"} {
		if !usableName(name) {
			t.Errorf("%q should be usable", name)
		}
	}
	for _, name := range []string{"", "has space", "slash/name", "new\nline", "ünï", ".", "..", "-", "_.-", strings.Repeat("a", maxToolNameBytes+1)} {
		if usableName(name) {
			t.Errorf("%q should not be usable", name)
		}
	}
	_, tally := reconcile(nil, []Found{readOnly("a", "x"), readOnly("a", "y"), readOnly("bad name", "z")})
	if tally.Found != 1 || tally.Ignored != 2 || tally.New != 1 {
		t.Errorf("tally = %+v", tally)
	}
}

func TestRefusalReasons(t *testing.T) {
	no := false
	cases := []struct {
		name string
		f    Found
		want string
	}{
		{"destructive wins", Found{Name: "t", ReadOnly: yes(), Destructive: yes()}, refusalDestructive},
		{"unmarked", Found{Name: "t"}, refusalUnmarked},
		{"read-only false", Found{Name: "t", ReadOnly: &no}, refusalUnmarked},
		{"long description", Found{Name: "t", ReadOnly: yes(), Description: strings.Repeat("é", maxDescriptionRunes+1)}, refusalDescription},
		{"fine", Found{Name: "t", ReadOnly: yes(), Description: strings.Repeat("é", maxDescriptionRunes)}, ""},
	}
	for _, c := range cases {
		if got := refusalOf(c.f, json.RawMessage(`{}`)); got != c.want {
			t.Errorf("%s: refusal = %q, want %q", c.name, got, c.want)
		}
	}
	if got := refusalOf(readOnly("t", ""), json.RawMessage(strings.Repeat(" ", maxSchemaBytes+1))); got != refusalSchema {
		t.Errorf("large schema refusal = %q", got)
	}
}

func TestANewReadOnlyToolIsOfferedAndADestructiveOneRefused(t *testing.T) {
	destructive := Found{Name: "wipe", ReadOnly: yes(), Destructive: yes()}
	changes, tally := reconcile(nil, []Found{readOnly("search", "s"), destructive, {Name: "plain"}})
	got := map[string]Change{}
	for _, c := range changes {
		got[c.Tool.Name] = c
	}
	if got["search"].Tool.State != ToolOffered || got["search"].Action != ActionList {
		t.Errorf("search = %+v", got["search"])
	}
	for _, name := range []string{"wipe", "plain"} {
		c := got[name]
		if c.Tool.State != ToolRefused || c.Action != ActionReject || c.Tool.Refusal == "" {
			t.Errorf("%s = %+v", name, c)
		}
	}
	if tally.New != 3 || tally.Found != 3 {
		t.Errorf("tally = %+v", tally)
	}
}

func TestNothingIsApprovedByDiscovery(t *testing.T) {
	changes, _ := reconcile(nil, []Found{readOnly("a", "x"), readOnly("b", "y")})
	for _, c := range changes {
		if c.Tool.State == ToolApproved || c.Tool.ApprovedBy != "" {
			t.Errorf("%s was approved by discovery", c.Tool.Name)
		}
	}
}

func TestAnUnchangedApprovedToolProducesNoChange(t *testing.T) {
	f := readOnly("search", "s")
	changes, tally := reconcile([]Tool{storedFrom(t, ToolApproved, f)}, []Found{f})
	if len(changes) != 0 || tally.Changed != 0 {
		t.Errorf("changes = %+v, tally = %+v", changes, tally)
	}
}

func TestAChangedDescriptionStopsAnApprovedTool(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolApproved, f)
	f.Description = "now it does something else"
	changes, tally := reconcile([]Tool{stored}, []Found{f})
	if len(changes) != 1 || tally.Changed != 1 {
		t.Fatalf("changes = %+v, tally = %+v", changes, tally)
	}
	c := changes[0]
	if c.Tool.State != ToolChanged || c.Action != ActionChange || c.Tool.Seen == nil {
		t.Fatalf("change = %+v", c)
	}
	if c.Tool.Pinned.Hash != stored.Pinned.Hash || c.Tool.Pinned.Description != "s" {
		t.Errorf("the pinned definition moved: %+v", c.Tool.Pinned)
	}
	if c.Tool.Seen.Description != f.Description || c.Tool.Seen.Hash == stored.Pinned.Hash {
		t.Errorf("seen = %+v", c.Tool.Seen)
	}
}

func TestAChangedSchemaStopsAnApprovedTool(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolApproved, f)
	f.InputSchema = json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"},"extra":{"type":"string"}}}`)
	changes, _ := reconcile([]Tool{stored}, []Found{f})
	if len(changes) != 1 || changes[0].Tool.State != ToolChanged {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestAChangedToolStaysChangedAndIsNotRewrittenWhenSeenAgain(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolApproved, f)
	f.Description = "v2"
	first, _ := reconcile([]Tool{stored}, []Found{f})
	again, tally := reconcile([]Tool{first[0].Tool}, []Found{f})
	if len(again) != 0 || tally.Changed != 0 {
		t.Errorf("a second discovery of the same change produced %+v", again)
	}
	f.Description = "v3"
	third, tally := reconcile([]Tool{first[0].Tool}, []Found{f})
	if len(third) != 1 || third[0].Tool.State != ToolChanged || third[0].Tool.Seen.Description != "v3" || third[0].Action != "" || tally.Changed != 0 {
		t.Errorf("a newer change = %+v, tally = %+v", third, tally)
	}
}

func TestAChangedToolReturnsToApprovedWhenTheServerGoesBack(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolApproved, f)
	changed := stored
	changed.State = ToolChanged
	changed.Seen = &Definition{Description: "v2", Hash: "other"}
	changes, _ := reconcile([]Tool{changed}, []Found{f})
	if len(changes) != 1 || changes[0].Tool.State != ToolApproved || changes[0].Action != ActionRestore || changes[0].Tool.Seen != nil {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestAnApprovedToolTheServerDroppedIsGoneAndComesBackUnchanged(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolApproved, f)
	changes, tally := reconcile([]Tool{stored}, nil)
	if len(changes) != 1 || changes[0].Tool.State != ToolGone || changes[0].Action != ActionDrop || tally.Gone != 1 {
		t.Fatalf("changes = %+v, tally = %+v", changes, tally)
	}
	back, _ := reconcile([]Tool{changes[0].Tool}, []Found{f})
	if len(back) != 1 || back[0].Tool.State != ToolApproved || back[0].Action != ActionRestore {
		t.Fatalf("back = %+v", back)
	}
	f.Description = "different"
	different, _ := reconcile([]Tool{changes[0].Tool}, []Found{f})
	if len(different) != 1 || different[0].Tool.State != ToolChanged {
		t.Errorf("a gone tool that returns different must be changed: %+v", different)
	}
}

func TestAnUnapprovedToolTheServerDroppedIsRemoved(t *testing.T) {
	f := readOnly("search", "s")
	changes, tally := reconcile([]Tool{storedFrom(t, ToolOffered, f)}, nil)
	if len(changes) != 1 || !changes[0].Remove || tally.Gone != 0 {
		t.Fatalf("changes = %+v, tally = %+v", changes, tally)
	}
}

func TestAnApprovedToolTheServerNowMarksDestructiveIsRefusedAndLosesItsApproval(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolApproved, f)
	f.Destructive = yes()
	changes, _ := reconcile([]Tool{stored}, []Found{f})
	if len(changes) != 1 {
		t.Fatalf("changes = %+v", changes)
	}
	c := changes[0]
	if c.Tool.State != ToolRefused || c.Action != ActionRefuse || c.Tool.ApprovedBy != "" || !c.Tool.ApprovedAt.IsZero() || c.Tool.Refusal != refusalDestructive {
		t.Errorf("change = %+v", c)
	}
}

func TestAnOfferedToolFollowsTheServerUntilApproved(t *testing.T) {
	f := readOnly("search", "s")
	stored := storedFrom(t, ToolOffered, f)
	f.Description = "v2"
	changes, tally := reconcile([]Tool{stored}, []Found{f})
	if len(changes) != 1 || changes[0].Tool.State != ToolOffered || changes[0].Tool.Pinned.Description != "v2" || tally.Changed != 0 {
		t.Fatalf("changes = %+v, tally = %+v", changes, tally)
	}
	f.Destructive = yes()
	changes, _ = reconcile([]Tool{stored}, []Found{f})
	if len(changes) != 1 || changes[0].Tool.State != ToolRefused || changes[0].Action != ActionRefuse {
		t.Fatalf("changes = %+v", changes)
	}
	refused := changes[0].Tool
	f.Destructive = nil
	changes, _ = reconcile([]Tool{refused}, []Found{f})
	if len(changes) != 1 || changes[0].Tool.State != ToolOffered || changes[0].Action != ActionOffer {
		t.Fatalf("a tool the server stopped refusing = %+v", changes)
	}
}

func TestReconcileOrdersChangesByName(t *testing.T) {
	changes, _ := reconcile(nil, []Found{readOnly("b", ""), readOnly("a", ""), readOnly("c", "")})
	var names []string
	for _, c := range changes {
		names = append(names, c.Tool.Name)
	}
	if strings.Join(names, ",") != "a,b,c" {
		t.Errorf("names = %v", names)
	}
}

func TestAChangedToolWithAnEmptyDescriptionStoresTheSeenDescriptionAsText(t *testing.T) {
	pinned := storedFrom(t, ToolApproved, readOnly("search", "finds things"))
	l, _ := listingOf([]Found{readOnly("search", "")})
	changed := pinned
	changed.State = ToolChanged
	changed.Seen = &l[0].def

	p := putParams(newID(), changed)
	if !p.SeenDescription.Valid || p.SeenDescription.String != "" {
		t.Fatalf("seen description = %+v, want valid empty text", p.SeenDescription)
	}
	if !p.SeenHash.Valid {
		t.Fatal("seen hash is not set")
	}

	plain := putParams(newID(), pinned)
	if plain.SeenDescription.Valid || plain.SeenHash.Valid {
		t.Fatalf("a tool without a seen definition stored one: %+v", plain)
	}
}
