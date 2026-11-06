package tools

import (
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	warnEveryStream   = "This selector matches every stream."
	warnEveryTrace    = "This query matches every trace."
	warnEveryDocument = "This query matches every document."
	warnLucene        = "This is a Lucene query; the explorer's Builder cannot edit it."
	warnNoTimeFilter  = "This query has no $__timeFilter, so it reads the whole table."
	warnNoLimit       = "This query has no LIMIT; the page shows at most 1,000 rows."
	timeFilterMacro   = "$__timeFilter"
	clickhouseLimitKw = "LIMIT"
)

func draftWarnings(dsType spec.DatasourceType, query string) []string {
	switch dsType {
	case spec.Loki:
		if matchers, ok := firstSelector(query); ok && everyMatcherMatchesAll(matchers) {
			return []string{warnEveryStream}
		}
	case spec.Tempo:
		if trimmed := strings.Join(strings.Fields(query), ""); trimmed == "{}" || trimmed == "{true}" {
			return []string{warnEveryTrace}
		}
	case spec.ClickHouse:
		return clickhouseWarnings(query)
	case spec.Elasticsearch, spec.OpenSearch:
		return searchWarnings(query)
	}
	return nil
}

func searchWarnings(query string) []string {
	form, body := elasticsearch.Classify(query, elasticsearch.QueryLanguageKQL)
	var warnings []string
	if form != elasticsearch.FormDSL && (body == "" || body == "*") {
		warnings = append(warnings, warnEveryDocument)
	}
	if form == elasticsearch.FormLucene {
		warnings = append(warnings, warnLucene)
	}
	return warnings
}

func firstSelector(query string) ([]string, bool) {
	start := strings.IndexByte(query, '{')
	if start < 0 {
		return nil, false
	}
	var matchers []string
	from := start + 1
	for i := from; i < len(query); i++ {
		switch query[i] {
		case '"', '`':
			end, ok := quotedEnd(query, i)
			if !ok {
				return nil, false
			}
			i = end
		case ',':
			matchers = append(matchers, query[from:i])
			from = i + 1
		case '}':
			return append(matchers, query[from:i]), true
		}
	}
	return nil, false
}

func quotedEnd(s string, start int) (int, bool) {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		switch {
		case quote == '"' && s[i] == '\\':
			i++
		case s[i] == quote:
			return i, true
		}
	}
	return 0, false
}

func everyMatcherMatchesAll(matchers []string) bool {
	for _, m := range matchers {
		if !matchesAll(strings.TrimSpace(m)) {
			return false
		}
	}
	return true
}

func matchesAll(matcher string) bool {
	op := strings.IndexAny(matcher, "=!")
	if op < 0 {
		return false
	}
	rest := matcher[op:]
	for _, candidate := range []struct{ op, value string }{{"=~", ".*"}, {"=~", ".+"}, {"!=", ""}, {"!~", ""}} {
		raw, ok := strings.CutPrefix(rest, candidate.op)
		if !ok {
			continue
		}
		value, err := strconv.Unquote(strings.TrimSpace(raw))
		if err == nil && value == candidate.value {
			return true
		}
	}
	return false
}

func clickhouseWarnings(query string) []string {
	tokens, err := macros.TokenizeClickHouse(query)
	if err != nil {
		return nil
	}
	timeFiltered, limited, depth := false, false, 0
	for _, tok := range tokens {
		word := query[tok.Start:tok.End]
		switch tok.Kind {
		case macros.TokenPunct:
			depth += parenDelta(word)
		case macros.TokenWord:
			timeFiltered = timeFiltered || strings.HasPrefix(word, timeFilterMacro)
			limited = limited || (depth == 0 && strings.EqualFold(word, clickhouseLimitKw))
		}
	}
	var warnings []string
	if !timeFiltered {
		warnings = append(warnings, warnNoTimeFilter)
	}
	if !limited {
		warnings = append(warnings, warnNoLimit)
	}
	return warnings
}

func parenDelta(punct string) int {
	switch punct {
	case "(":
		return 1
	case ")":
		return -1
	}
	return 0
}
