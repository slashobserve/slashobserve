package spec

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
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
	Nested  []Reference
	Start   int
	End     int
}

type MacroFormats int

const (
	NoFormats MacroFormats = iota
	VariableFormatsAllowed
	TimeFormatsAllowed
)

type MacroArgs int

const (
	NoArgs MacroArgs = iota
	OneColumn
	TwoColumns
	ConditionAndVariable
)

type Macro struct {
	Name         string
	Args         MacroArgs
	Formats      MacroFormats
	Global       bool
	PerUser      bool
	PerDashboard bool
}

func (m Macro) Function() bool {
	return m.Args != NoArgs
}

type macroDef struct {
	Macro
	datasources []DatasourceType
}

var (
	bothDatasources = []DatasourceType{Prometheus, ClickHouse}
	clickHouseOnly  = []DatasourceType{ClickHouse}
)

var macroTable = []macroDef{
	{Macro{Name: "__interval"}, bothDatasources},
	{Macro{Name: "__interval_ms"}, bothDatasources},
	{Macro{Name: "__range"}, []DatasourceType{Prometheus}},
	{Macro{Name: "__range_s"}, bothDatasources},
	{Macro{Name: "__range_ms"}, bothDatasources},
	{Macro{Name: "__rate_interval"}, []DatasourceType{Prometheus}},
	{Macro{Name: "__from", Formats: TimeFormatsAllowed}, bothDatasources},
	{Macro{Name: "__to", Formats: TimeFormatsAllowed}, bothDatasources},
	{Macro{Name: "__dashboard", Formats: VariableFormatsAllowed, Global: true, PerDashboard: true}, bothDatasources},
	{Macro{Name: "__user.login", Formats: VariableFormatsAllowed, Global: true, PerUser: true}, bothDatasources},
	{Macro{Name: "__user.email", Formats: VariableFormatsAllowed, Global: true, PerUser: true}, bothDatasources},
	{Macro{Name: "__user.name", Formats: VariableFormatsAllowed, Global: true, PerUser: true}, bothDatasources},
	{Macro{Name: "__user.id", Formats: VariableFormatsAllowed, Global: true, PerUser: true}, bothDatasources},
	{Macro{Name: "__timeFilter", Args: OneColumn}, clickHouseOnly},
	{Macro{Name: "__timeFilter_ms", Args: OneColumn}, clickHouseOnly},
	{Macro{Name: "__timeGroup", Args: OneColumn}, clickHouseOnly},
	{Macro{Name: "__dateFilter", Args: OneColumn}, clickHouseOnly},
	{Macro{Name: "__dateTimeFilter", Args: TwoColumns}, clickHouseOnly},
	{Macro{Name: "__dt", Args: TwoColumns}, clickHouseOnly},
	{Macro{Name: "__fromTime"}, clickHouseOnly},
	{Macro{Name: "__toTime"}, clickHouseOnly},
	{Macro{Name: "__fromTime_ms"}, clickHouseOnly},
	{Macro{Name: "__toTime_ms"}, clickHouseOnly},
	{Macro{Name: "__interval_s"}, clickHouseOnly},
	{Macro{Name: "__timeInterval", Args: OneColumn}, clickHouseOnly},
	{Macro{Name: "__timeInterval_ms", Args: OneColumn}, clickHouseOnly},
	{Macro{Name: "__conditionalAll", Args: ConditionAndVariable}, clickHouseOnly},
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

func LookupMacro(name string) (Macro, bool) {
	idx := slices.IndexFunc(macroTable, func(m macroDef) bool { return m.Name == name })
	if idx < 0 {
		return Macro{}, false
	}
	return macroTable[idx].Macro, true
}

func (m Macro) CheckFormat(format string) error {
	switch m.Formats {
	case VariableFormatsAllowed:
		if !slices.Contains(VariableFormats(), format) {
			return fmt.Errorf("unknown format %q for $%s; use one of %s", format, m.Name, strings.Join(VariableFormats(), ", "))
		}
	case TimeFormatsAllowed:
		if !ValidTimeMacroFormat(format) {
			return fmt.Errorf("invalid date format %q for $%s", format, m.Name)
		}
	default:
		return fmt.Errorf("macro $%s does not take a format", m.Name)
	}
	return nil
}

func validFormatFor(name, format string) bool {
	if macro, ok := LookupMacro(name); ok && macro.Formats == TimeFormatsAllowed {
		return ValidTimeMacroFormat(format)
	}
	return validFormat(format)
}

func VariableFormats() []string {
	return []string{"csv", "doublequote", "pipe", "raw", "regex", "singlequote", "sqlstring"}
}

const maxScanWork = 1 << 22

type ScanError struct {
	Prefix string
	Offset int
}

func (e *ScanError) Error() string {
	return e.Prefix + strconv.Itoa(e.Offset)
}

func scanErrorf(offset int, format string, args ...any) *ScanError {
	return &ScanError{Prefix: fmt.Sprintf(format, args...), Offset: offset}
}

func ScanQuery(query string) ([]Reference, error) {
	budget := maxScanWork
	return scanQuery(query, &budget, false)
}

func FlattenReferences(refs []Reference) []Reference {
	var out []Reference
	for _, ref := range refs {
		out = append(out, ref)
		out = append(out, FlattenReferences(ref.Nested)...)
	}
	return out
}

func scanQuery(query string, budget *int, inArgs bool) ([]Reference, error) {
	var refs []Reference
	for i := 0; i < len(query); {
		if query[i] != '$' {
			i++
			continue
		}
		ref, next, err := scanReference(query, i, budget, inArgs)
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

func scanReference(query string, start int, budget *int, inArgs bool) (*Reference, int, error) {
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
			return nil, 0, scanErrorf(ref.End, "too many unclosed \"(\" in macro arguments; stopped at offset ")
		}
		if !ok && !knownMacro(ref.Name) {
			return ref, ref.End, nil
		}
		if !ok {
			return nil, 0, scanErrorf(ref.End, "unclosed \"(\" at offset ")
		}
		open := ref.End
		ref.Args, ref.HasArgs, ref.End = args, true, end
		if macro, known := LookupMacro(ref.Name); known && macro.Args == ConditionAndVariable && !inArgs {
			nested, err := scanQuery(args, budget, true)
			if err != nil {
				var inner *ScanError
				if errors.As(err, &inner) {
					return nil, 0, &ScanError{Prefix: fmt.Sprintf("in the arguments of $%s: %s", ref.Name, inner.Prefix), Offset: inner.Offset + open + 1}
				}
				return nil, 0, fmt.Errorf("in the arguments of $%s: %w", ref.Name, err)
			}
			for i := range nested {
				nested[i].Start += open + 1
				nested[i].End += open + 1
			}
			ref.Nested = nested
		}
	}
	return ref, ref.End, nil
}

type MacroArguments struct {
	Values          []string
	ConditionOffset int
}

func (m Macro) ParseArguments(ref Reference) (MacroArguments, error) {
	switch m.Args {
	case OneColumn:
		switch args := strings.TrimSpace(ref.Args); {
		case args == "":
			return MacroArguments{}, fmt.Errorf("macro $%s needs arguments, for example $%s(column)", m.Name, m.Name)
		case !IsColumnArg(args):
			return MacroArguments{}, fmt.Errorf("macro $%s argument must be a column name such as column or table.column", m.Name)
		default:
			return MacroArguments{Values: []string{args}}, nil
		}
	case TwoColumns:
		parts := SplitMacroArgs(ref.Args)
		if len(parts) != 2 || !IsColumnArg(parts[0]) || !IsColumnArg(parts[1]) {
			return MacroArguments{}, fmt.Errorf("macro $%s needs two column names, for example $%s(date_column, time_column)", m.Name, m.Name)
		}
		return MacroArguments{Values: parts}, nil
	case ConditionAndVariable:
		return parseConditionAndVariable(m, ref)
	}
	if ref.HasArgs {
		return MacroArguments{}, fmt.Errorf("macro $%s does not take arguments; remove the parentheses", m.Name)
	}
	return MacroArguments{}, nil
}

func parseConditionAndVariable(m Macro, ref Reference) (MacroArguments, error) {
	parts := SplitMacroArgs(ref.Args)
	if len(parts) != 2 || parts[0] == "" {
		return MacroArguments{}, fmt.Errorf("macro $%s needs a condition and a variable, for example $%s(pod IN (${pod:singlequote}), $pod)", m.Name, m.Name)
	}
	budget := maxScanWork
	variable, err := scanQuery(parts[1], &budget, true)
	if err != nil || len(variable) != 1 || variable[0].Kind != VariableRef || variable[0].Format != "" || variable[0].Start != 0 || variable[0].End != len(parts[1]) {
		return MacroArguments{}, fmt.Errorf("the second argument of $%s must be one variable reference such as $pod", m.Name)
	}
	argsAt := ref.Start + 1 + len(ref.Name) + 1
	conditionAt := len(ref.Args) - len(strings.TrimLeftFunc(ref.Args, unicode.IsSpace))
	conditionEnd := conditionAt + len(parts[0])
	for _, nested := range ref.Nested {
		at := nested.Start - argsAt
		if nested.Kind == MacroRef && nested.Name == m.Name && at >= conditionAt && at < conditionEnd && !insideOpaque(ref.Args, at) {
			return MacroArguments{}, fmt.Errorf("$%s cannot be nested", m.Name)
		}
	}
	return MacroArguments{Values: []string{parts[0], variable[0].Name}, ConditionOffset: conditionAt}, nil
}

func insideOpaque(s string, offset int) bool {
	for i := 0; i <= offset && i < len(s); i++ {
		if end, skipped := skipOpaque(s, i); skipped {
			if offset <= end {
				return true
			}
			i = end
		}
	}
	return false
}

func SplitMacroArgs(args string) []string {
	var parts []string
	depth, from := 0, 0
	for i := 0; i < len(args); i++ {
		if end, skipped := skipOpaque(args, i); skipped {
			i = end
			continue
		}
		switch c := args[i]; {
		case c == '$' && strings.HasPrefix(args[i+1:], "{"):
			if end := strings.IndexByte(args[i:], '}'); end >= 0 {
				i += end
			} else {
				i = len(args)
			}
		case c == '(', c == '[', c == '{':
			depth++
		case c == ')', c == ']', c == '}':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(args[from:i]))
			from = i + 1
		}
	}
	return append(parts, strings.TrimSpace(args[from:]))
}

func scanBraced(query string, start int) (*Reference, int, error) {
	closing := strings.IndexByte(query[start:], '}')
	if closing < 0 {
		return nil, 0, scanErrorf(start, "unterminated \"${\" at offset ")
	}
	end := start + closing + 1
	name, format, hasFormat := strings.Cut(query[start+2:end-1], ":")
	if !hasFormat && isDigits(name) {
		return nil, end, nil
	}
	if !validReferenceName(name) || (hasFormat && !validFormatFor(name, format)) {
		return nil, 0, scanErrorf(start, "invalid reference %q at offset ", query[start:end])
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
		if end, skipped := skipOpaque(s, i); skipped {
			i = end
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

func skipOpaque(s string, i int) (end int, skipped bool) {
	switch {
	case isLineComment(s, i):
		return skipLine(s, i), true
	case strings.HasPrefix(s[i:], "/*"):
		return skipBlockComment(s, i), true
	case s[i] == '\'', s[i] == '"', s[i] == '`':
		return skipQuoted(s, i), true
	}
	return i, false
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
