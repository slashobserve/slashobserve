package macros

import (
	"slices"
	"strings"
)

var (
	fromListEnders = map[string]bool{
		"SELECT": true, "WITH": true, "WHERE": true, "PREWHERE": true, "GROUP": true, "HAVING": true,
		"ORDER": true, "LIMIT": true, "OFFSET": true, "WINDOW": true, "QUALIFY": true,
		"UNION": true, "INTERSECT": true, "EXCEPT": true, "SETTINGS": true, "FORMAT": true,
		"ON": true, "USING": true,
	}
	fromInsideFunction = map[string]bool{"extract": true, "trim": true, "substring": true, "substr": true, "mid": true}
)

type tableScope struct {
	functionCall bool
	fromList     bool
}

type tableScan struct {
	query  string
	sig    []Token
	ctes   []string
	scopes []tableScope
	tables []string
}

func ClickHouseTables(text string) []string {
	tokens, err := TokenizeClickHouse(text)
	if err != nil {
		return nil
	}
	s := &tableScan{query: text, scopes: []tableScope{{}}}
	for _, t := range tokens {
		if t.Kind != TokenWhitespace && t.Kind != TokenComment {
			s.sig = append(s.sig, t)
		}
	}
	s.ctes = s.cteNames()
	s.scan()
	return s.tables
}

func (s *tableScan) isPunct(i int, text string) bool {
	return i >= 0 && i < len(s.sig) && s.sig[i].Kind == TokenPunct && s.query[s.sig[i].Start:s.sig[i].End] == text
}

func (s *tableScan) keyword(i int) string {
	if i < 0 || i >= len(s.sig) || s.sig[i].Kind != TokenWord || s.isPunct(i-1, ".") {
		return ""
	}
	return strings.ToUpper(s.query[s.sig[i].Start:s.sig[i].End])
}

func (s *tableScan) name(i int) (string, bool) {
	if i < 0 || i >= len(s.sig) {
		return "", false
	}
	t := s.sig[i]
	switch t.Kind {
	case TokenWord:
		return s.query[t.Start:t.End], true
	case TokenQuotedIdent:
		return t.Name, t.NameExact
	}
	return "", false
}

func (s *tableScan) isName(i int) bool {
	return i >= 0 && i < len(s.sig) && (s.sig[i].Kind == TokenWord || s.sig[i].Kind == TokenQuotedIdent)
}

func (s *tableScan) cteNames() []string {
	var names []string
	for i := range s.sig {
		if !s.isName(i) || s.keyword(i+1) != "AS" || !s.isPunct(i+2, "(") {
			continue
		}
		if prev := s.keyword(i - 1); prev != "WITH" && prev != "RECURSIVE" && !s.isPunct(i-1, ",") {
			continue
		}
		if name, ok := s.name(i); ok {
			names = append(names, name)
		}
	}
	return names
}

func (s *tableScan) top() *tableScope { return &s.scopes[len(s.scopes)-1] }

func (s *tableScan) scan() {
	expectTable := false
	for i := 0; i < len(s.sig); i++ {
		switch {
		case s.isPunct(i, "("):
			expectTable = false
			prev, _ := s.name(i - 1)
			s.scopes = append(s.scopes, tableScope{functionCall: s.keyword(i-1) != "" && fromInsideFunction[strings.ToLower(prev)]})
		case s.isPunct(i, ")"):
			expectTable = false
			if len(s.scopes) > 1 {
				s.scopes = s.scopes[:len(s.scopes)-1]
			}
		case s.isPunct(i, ","):
			expectTable = s.top().fromList
		case expectTable && s.isName(i):
			expectTable = false
			i = s.table(i)
		default:
			expectTable = s.keywordEffect(i)
		}
	}
}

func (s *tableScan) keywordEffect(i int) bool {
	switch word := s.keyword(i); {
	case word == "FROM" && !s.top().functionCall:
		s.top().fromList = true
		return true
	case word == "JOIN" && s.keyword(i-1) == "ARRAY":
		s.top().fromList = false
		return false
	case word == "JOIN":
		s.top().fromList = true
		return true
	case fromListEnders[word]:
		s.top().fromList = false
		return false
	}
	return false
}

func (s *tableScan) table(i int) int {
	parts := []string{}
	exact := true
	end := i
	for {
		name, ok := s.name(end)
		exact = exact && ok
		parts = append(parts, name)
		if !s.isPunct(end+1, ".") || !s.isName(end+2) {
			break
		}
		end += 2
	}
	if s.isPunct(end+1, "(") || !exact || len(parts) > 2 {
		return end
	}
	if full := strings.Join(parts, "."); !(len(parts) == 1 && slices.Contains(s.ctes, parts[0])) && !slices.Contains(s.tables, full) {
		s.tables = append(s.tables, full)
	}
	return end
}
