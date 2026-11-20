package filter

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var dateLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

var orderedOps = map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}

type cond struct {
	column string
	op     string
	arg    any
	cast   string
}

func (c cond) sql(b *builder) string {
	return c.column + " " + c.op + " " + b.add(c.arg) + c.cast
}

func (f *Field) equal(text string) ([]cond, error) {
	switch f.Type {
	case TypeTarget:
		return targetConds(text)
	case TypeEnum:
		return f.single(f.enumValue(text))
	case TypeInt:
		return f.single(f.intValue(text))
	case TypeUUID:
		return f.single(f.uuidValue(text))
	case TypeIP:
		return f.single(f.ipValue(text))
	case TypeDate:
		return f.single(f.dateValue(text, "="))
	default:
		return []cond{{column: f.column, op: "=", arg: text}}, nil
	}
}

func (f *Field) single(c cond, err error) ([]cond, error) {
	if err != nil {
		return nil, err
	}
	return []cond{c}, nil
}

func (f *Field) ordered(key, text string) (cond, error) {
	op, ok := orderedOps[key]
	if !ok {
		return cond{}, fmt.Errorf("range accepts gt, gte, lt and lte, not %q", key)
	}
	switch f.Type {
	case TypeText:
		return cond{column: f.column, op: op, arg: text}, nil
	case TypeInt:
		c, err := f.intValue(text)
		c.op = op
		return c, err
	case TypeDate:
		return f.dateValue(text, op)
	default:
		return cond{}, fmt.Errorf("range is not supported on %s fields such as %s", f.Type, f.Name)
	}
}

func (f *Field) enumValue(text string) (cond, error) {
	if !slices.Contains(f.Values, text) {
		hint := "use one of: " + strings.Join(f.Values, ", ")
		if s := nearest(text, f.Values); s != "" {
			hint = fmt.Sprintf("did you mean %q?", s)
		}
		return cond{}, f.badValue(text, hint)
	}
	return cond{column: f.column, op: "=", arg: text}, nil
}

func (f *Field) intValue(text string) (cond, error) {
	n, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return cond{}, f.badValue(text, "a whole number is expected")
	}
	return cond{column: f.column, op: "=", arg: int32(n)}, nil
}

func (f *Field) uuidValue(text string) (cond, error) {
	id, err := uuid.Parse(text)
	if err != nil {
		return cond{}, f.badValue(text, "a UUID is expected")
	}
	return cond{column: f.column, op: "=", arg: id.String(), cast: "::uuid"}, nil
}

func (f *Field) dateValue(text, op string) (cond, error) {
	for _, layout := range dateLayouts {
		if at, err := time.Parse(layout, text); err == nil {
			return cond{column: f.column, op: op, arg: at.UTC()}, nil
		}
	}
	return cond{}, f.badValue(text, "a time such as 2026-11-01T10:00:00Z or a date such as 2026-11-01 is expected")
}

func (f *Field) ipValue(text string) (cond, error) {
	const hint = "an IP address or a CIDR such as 10.4.0.0/16 is expected"
	if strings.Contains(text, "/") {
		p, err := netip.ParsePrefix(text)
		if err != nil || p.Addr().Zone() != "" {
			return cond{}, f.badValue(text, hint)
		}
		return cond{column: f.column, op: "<<=", arg: p.Masked().String(), cast: "::inet"}, nil
	}
	a, err := netip.ParseAddr(text)
	if err != nil || a.Zone() != "" {
		return cond{}, f.badValue(text, hint)
	}
	return cond{column: f.column, op: "=", arg: a.String(), cast: "::inet"}, nil
}

func targetConds(text string) ([]cond, error) {
	kind, id, hasID := strings.Cut(text, "/")
	switch {
	case kind == "":
		return nil, errors.New("target needs a kind, as in dashboard/<id>")
	case hasID && id == "":
		return nil, errors.New("target is missing an id after the /")
	case hasID:
		return []cond{{column: "target_kind", op: "=", arg: kind}, {column: "target_id", op: "=", arg: id}}, nil
	default:
		return []cond{{column: "target_kind", op: "=", arg: kind}}, nil
	}
}

func (f *Field) badValue(text, hint string) error {
	return fmt.Errorf("bad value %q for %s: %s", text, f.Name, hint)
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

type pieceKind int

const (
	pieceText pieceKind = iota
	pieceMany
	pieceOne
)

type wildcardPiece struct {
	kind pieceKind
	text string
}

func likePattern(pieces []wildcardPiece) string {
	var b strings.Builder
	for _, p := range pieces {
		switch p.kind {
		case pieceMany:
			b.WriteByte('%')
		case pieceOne:
			b.WriteByte('_')
		default:
			b.WriteString(likeEscaper.Replace(p.text))
		}
	}
	return b.String()
}

func elasticsearchWildcard(pattern string) []wildcardPiece {
	var pieces []wildcardPiece
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			pieces = append(pieces, wildcardPiece{text: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case c == '\\' && i+1 < len(pattern):
			i++
			lit.WriteByte(pattern[i])
		case c == '*' || c == '?':
			flush()
			pieces = append(pieces, wildcardPiece{kind: map[byte]pieceKind{'*': pieceMany, '?': pieceOne}[c]})
		default:
			lit.WriteByte(c)
		}
	}
	flush()
	return pieces
}
