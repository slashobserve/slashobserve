package contextsources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTools            = 200
	maxDescriptionRunes = 4000
	maxSchemaBytes      = 32 << 10
	maxToolNameBytes    = 128

	refusalDestructive = "The server marks this tool as destructive."
	refusalUnmarked    = "The server does not mark this tool as read-only."
	refusalDescription = "The description is longer than 4000 characters."
	refusalSchema      = "The input schema is larger than 32 KiB."
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Found struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    *bool
	Destructive *bool
}

type Definition struct {
	Description string
	InputSchema json.RawMessage
	Hash        string
}

type Tool struct {
	Name       string
	State      ToolState
	Refusal    string
	Pinned     Definition
	Seen       *Definition
	ApprovedBy string
	ApprovedAt time.Time
	UpdatedAt  time.Time
}

type Change struct {
	Tool   Tool
	Action ToolAction
	Remove bool
}

type Tally struct {
	Found   int `json:"found"`
	New     int `json:"new"`
	Changed int `json:"changed"`
	Gone    int `json:"gone"`
	Ignored int `json:"ignored"`
}

func canonicalSchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{"type":"object"}`), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil {
		return json.RawMessage(`{"type":"object"}`), nil
	}
	out, err := json.Marshal(value)
	return out, err
}

func hashOf(name, description string, schema json.RawMessage) string {
	sum := sha256.New()
	for _, part := range []string{name, description, string(schema)} {
		sum.Write([]byte(part))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func refusalOf(f Found, schema json.RawMessage) string {
	switch {
	case f.Destructive != nil && *f.Destructive:
		return refusalDestructive
	case f.ReadOnly == nil || !*f.ReadOnly:
		return refusalUnmarked
	case utf8.RuneCountInString(f.Description) > maxDescriptionRunes:
		return refusalDescription
	case len(schema) > maxSchemaBytes:
		return refusalSchema
	}
	return ""
}

func usableName(name string) bool {
	return len(name) <= maxToolNameBytes && toolNamePattern.MatchString(name) && strings.ContainsFunc(name, isLetterOrDigit)
}

func isLetterOrDigit(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

type listing struct {
	found   Found
	def     Definition
	refusal string
}

func listingOf(found []Found) ([]listing, int) {
	var out []listing
	seen := map[string]bool{}
	ignored := 0
	for _, f := range found {
		if !usableName(f.Name) || seen[f.Name] {
			ignored++
			continue
		}
		seen[f.Name] = true
		schema, err := canonicalSchema(f.InputSchema)
		if err != nil {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, listing{found: f, def: Definition{Description: f.Description, InputSchema: schema, Hash: hashOf(f.Name, f.Description, schema)}, refusal: refusalOf(f, schema)})
	}
	slices.SortFunc(out, func(a, b listing) int { return strings.Compare(a.found.Name, b.found.Name) })
	return out, ignored
}

func reconcile(stored []Tool, found []Found) ([]Change, Tally) {
	listed, ignored := listingOf(found)
	byName := make(map[string]Tool, len(stored))
	for _, t := range stored {
		byName[t.Name] = t
	}
	var changes []Change
	for _, l := range listed {
		cur, known := byName[l.found.Name]
		delete(byName, l.found.Name)
		if c, ok := reconcileOne(cur, known, l); ok {
			changes = append(changes, c)
		}
	}
	for _, t := range slices.SortedFunc(maps.Values(byName), func(a, b Tool) int { return strings.Compare(a.Name, b.Name) }) {
		if c, ok := reconcileMissing(t); ok {
			changes = append(changes, c)
		}
	}
	return changes, tallyOf(changes, len(listed), ignored)
}

func reconcileOne(cur Tool, known bool, l listing) (Change, bool) {
	if !known {
		t := Tool{Name: l.found.Name, Pinned: l.def, State: ToolOffered}
		action := ActionList
		if l.refusal != "" {
			t.State, t.Refusal, action = ToolRefused, l.refusal, ActionReject
		}
		return Change{Tool: t, Action: action}, true
	}
	switch cur.State {
	case ToolOffered, ToolRefused:
		return reconcileUnapproved(cur, l)
	}
	return reconcileApproved(cur, l)
}

func reconcileUnapproved(cur Tool, l listing) (Change, bool) {
	next := cur
	next.Pinned = l.def
	next.Refusal = l.refusal
	next.State = ToolOffered
	action := ToolAction("")
	switch {
	case l.refusal != "":
		next.State = ToolRefused
		if cur.State == ToolOffered {
			action = ActionRefuse
		}
	case cur.State == ToolRefused:
		action = ActionOffer
	}
	if next.State == cur.State && next.Refusal == cur.Refusal && next.Pinned.Hash == cur.Pinned.Hash {
		return Change{}, false
	}
	return Change{Tool: next, Action: action}, true
}

func reconcileApproved(cur Tool, l listing) (Change, bool) {
	next := cur
	if l.refusal != "" {
		next.State, next.Refusal, next.Pinned, next.Seen = ToolRefused, l.refusal, l.def, nil
		next.ApprovedBy, next.ApprovedAt = "", time.Time{}
		return Change{Tool: next, Action: ActionRefuse}, true
	}
	if l.def.Hash == cur.Pinned.Hash {
		if cur.State == ToolApproved {
			return Change{}, false
		}
		next.State, next.Seen = ToolApproved, nil
		return Change{Tool: next, Action: ActionRestore}, true
	}
	if cur.State == ToolChanged && cur.Seen != nil && cur.Seen.Hash == l.def.Hash {
		return Change{}, false
	}
	seen := l.def
	next.State, next.Seen = ToolChanged, &seen
	if cur.State == ToolChanged {
		return Change{Tool: next}, true
	}
	return Change{Tool: next, Action: ActionChange}, true
}

func reconcileMissing(cur Tool) (Change, bool) {
	switch cur.State {
	case ToolOffered, ToolRefused:
		return Change{Tool: cur, Remove: true}, true
	case ToolApproved, ToolChanged:
		next := cur
		next.State, next.Seen = ToolGone, nil
		return Change{Tool: next, Action: ActionDrop}, true
	}
	return Change{}, false
}

func tallyOf(changes []Change, listed, ignored int) Tally {
	t := Tally{Found: listed, Ignored: ignored}
	for _, c := range changes {
		switch c.Action {
		case ActionList, ActionReject:
			t.New++
		case ActionChange:
			t.Changed++
		case ActionDrop:
			t.Gone++
		}
	}
	return t
}
