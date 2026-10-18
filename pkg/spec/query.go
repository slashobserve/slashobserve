package spec

import (
	"fmt"
	"slices"
	"strings"
)

type ReferenceKind int

const (
	VariableRef ReferenceKind = iota + 1
	MacroRef
)

type Reference struct {
	Kind    ReferenceKind
	Name    string
	Format  string
	Args    string
	HasArgs bool
	Start   int
	End     int
}

type MacroFormats int

const (
	NoFormats MacroFormats = iota
	VariableFormatsAllowed
	TimeFormatsAllowed
)

type Macro struct {
	Name     string
	Function bool
	Formats  MacroFormats
}

type macroDef struct {
	Macro
	datasources []DatasourceType
}

var bothDatasources = []DatasourceType{Prometheus, ClickHouse}

var macroTable = []macroDef{
	{Macro{"__interval", false, NoFormats}, bothDatasources},
	{Macro{"__interval_ms", false, NoFormats}, bothDatasources},
	{Macro{"__range", false, NoFormats}, []DatasourceType{Prometheus}},
	{Macro{"__range_s", false, NoFormats}, bothDatasources},
	{Macro{"__range_ms", false, NoFormats}, bothDatasources},
	{Macro{"__rate_interval", false, NoFormats}, []DatasourceType{Prometheus}},
	{Macro{"__from", false, TimeFormatsAllowed}, bothDatasources},
	{Macro{"__to", false, TimeFormatsAllowed}, bothDatasources},
	{Macro{"__dashboard", false, VariableFormatsAllowed}, bothDatasources},
	{Macro{"__user.login", false, VariableFormatsAllowed}, bothDatasources},
	{Macro{"__user.email", false, VariableFormatsAllowed}, bothDatasources},
	{Macro{"__user.name", false, VariableFormatsAllowed}, bothDatasources},
	{Macro{"__user.id", false, VariableFormatsAllowed}, bothDatasources},
	{Macro{"__timeFilter", true, NoFormats}, []DatasourceType{ClickHouse}},
	{Macro{"__timeGroup", true, NoFormats}, []DatasourceType{ClickHouse}},
}

func Macros(t DatasourceType) []Macro {
	var out []Macro
	for _, m := range macroTable {
		if slices.Contains(m.datasources, t) {
			out = append(out, m.Macro)
		}
	}
	return out
}

func knownMacro(name string) bool {
	return slices.ContainsFunc(macroTable, func(m macroDef) bool { return m.Name == name })
}

func lookupMacro(name string) (Macro, bool) {
	idx := slices.IndexFunc(macroTable, func(m macroDef) bool { return m.Name == name })
	if idx < 0 {
		return Macro{}, false
	}
	return macroTable[idx].Macro, true
}

func validFormatFor(name, format string) bool {
	if macro, ok := lookupMacro(name); ok && macro.Formats == TimeFormatsAllowed {
		return ValidTimeMacroFormat(format)
	}
	return validFormat(format)
}

func VariableFormats() []string {
	return []string{"csv", "doublequote", "pipe", "raw", "regex", "singlequote", "sqlstring"}
}

const maxScanWork = 1 << 22

func ScanQuery(query string) ([]Reference, error) {
	var refs []Reference
	budget := maxScanWork
	for i := 0; i < len(query); {
		if query[i] != '$' {
			i++
			continue
		}
		ref, next, err := scanReference(query, i, &budget)
		if err != nil {
			return nil, err
		}
		if ref != nil {
			refs = append(refs, *ref)
		}
		i = next
	}
	return refs, nil
}

func scanReference(query string, start int, budget *int) (*Reference, int, error) {
	rest := query[start+1:]
	if strings.HasPrefix(rest, "{") {
		return scanBraced(query, start)
	}
	n := identLen(rest)
	if n == 0 {
		return nil, start + 1, nil
	}
	ref := &Reference{Name: rest[:n], Start: start, End: start + 1 + n}
	ref.Kind = referenceKind(ref.Name)
	if ref.Kind == MacroRef && strings.HasPrefix(rest[n:], "(") {
		args, end, ok := matchParen(query, ref.End)
		if *budget -= matchedBytes(query, ref.End, end, ok); *budget < 0 {
			return nil, 0, fmt.Errorf("too many unclosed \"(\" in macro arguments; stopped at offset %d", ref.End)
		}
		if !ok && !knownMacro(ref.Name) {
			return ref, ref.End, nil
		}
		if !ok {
			return nil, 0, fmt.Errorf("unclosed \"(\" at offset %d", ref.End)
		}
		ref.Args, ref.HasArgs, ref.End = args, true, end
	}
	return ref, ref.End, nil
}

func scanBraced(query string, start int) (*Reference, int, error) {
	closing := strings.IndexByte(query[start:], '}')
	if closing < 0 {
		return nil, 0, fmt.Errorf("unterminated \"${\" at offset %d", start)
	}
	end := start + closing + 1
	name, format, hasFormat := strings.Cut(query[start+2:end-1], ":")
	if !hasFormat && isDigits(name) {
		return nil, end, nil
	}
	if !validReferenceName(name) || (hasFormat && !validFormatFor(name, format)) {
		return nil, 0, fmt.Errorf("invalid reference %q at offset %d", query[start:end], start)
	}
	return &Reference{Kind: referenceKind(name), Name: name, Format: format, Start: start, End: end}, end, nil
}

func validReferenceName(name string) bool {
	return validIdent(name) || (strings.Contains(name, ".") && knownMacro(name))
}

func referenceKind(name string) ReferenceKind {
	if strings.HasPrefix(name, "__") {
		return MacroRef
	}
	return VariableRef
}

func matchedBytes(query string, open, end int, ok bool) int {
	if ok {
		return end - open
	}
	return len(query) - open
}

func matchParen(s string, open int) (args string, end int, ok bool) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch {
		case isLineComment(s, i):
			i = skipLine(s, i)
		case strings.HasPrefix(s[i:], "/*"):
			i = skipBlockComment(s, i)
		case s[i] == '\'', s[i] == '"', s[i] == '`':
			i = skipQuoted(s, i)
		case s[i] == '(':
			depth++
		case s[i] == ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

func skipLine(s string, from int) int {
	if end := strings.IndexByte(s[from:], '\n'); end >= 0 {
		return from + end
	}
	return len(s)
}

func isLineComment(s string, at int) bool {
	rest := s[at:]
	if strings.HasPrefix(rest, "--") || strings.HasPrefix(rest, "//") {
		return true
	}
	return rest[0] == '#' && len(rest) > 1 && (rest[1] == ' ' || rest[1] == '!')
}

func skipBlockComment(s string, open int) int {
	depth := 0
	for j := open; j < len(s)-1; {
		switch {
		case s[j] == '/' && s[j+1] == '*':
			depth++
			j += 2
		case s[j] == '*' && s[j+1] == '/':
			depth--
			j += 2
			if depth == 0 {
				return j - 1
			}
		default:
			j++
		}
	}
	return len(s)
}

func skipQuoted(s string, open int) int {
	quote := s[open]
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if quote != '`' {
				i++
			}
		case quote:
			return i
		}
	}
	return len(s)
}

func identLen(s string) int {
	n := 0
	for n < len(s) && isIdentByte(s[n], n == 0) {
		n++
	}
	return n
}

func isIdentByte(b byte, first bool) bool {
	switch {
	case b == '_', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return !first
	}
	return false
}

func validIdent(s string) bool {
	return s != "" && identLen(s) == len(s)
}

func validFormat(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}
