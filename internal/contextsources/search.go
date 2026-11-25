package contextsources

import (
	"cmp"
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode"
)

const (
	scoreNameWord    = 6.0
	scoreNamePrefix  = 3.0
	scoreParameter   = 3.0
	scoreDescription = 1.0
	scoreWholeName   = 5.0
	minPrefixLength  = 3
	minStemLength    = 4
)

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "in": true, "on": true, "at": true, "for": true, "to": true,
	"from": true, "with": true, "by": true, "and": true, "or": true, "is": true, "are": true, "be": true,
	"it": true, "its": true, "this": true, "that": true, "my": true, "me": true, "i": true, "we": true,
	"do": true, "does": true, "how": true, "what": true, "which": true, "who": true, "why": true,
	"can": true, "all": true, "any": true, "some": true, "as": true, "into": true,
}

type searchable struct {
	source Source
	tool   Tool
	name   []string
	params []string
	words  map[string]bool
	whole  string
}

type scored struct {
	searchable
	score float64
}

func words(text string) []string {
	var out []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			out = append(out, stem(string(current)))
			current = current[:0]
		}
	}
	runes := []rune(text)
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
			flush()
		}
		current = append(current, unicode.ToLower(r))
	}
	flush()
	return out
}

func stem(word string) string {
	switch {
	case len(word) >= minStemLength+2 && strings.HasSuffix(word, "ies"):
		return word[:len(word)-3] + "y"
	case len(word) >= minStemLength+3 && strings.HasSuffix(word, "ing"):
		return undouble(word[:len(word)-3])
	case len(word) >= minStemLength+1 && hasSibilantPlural(word):
		return word[:len(word)-2]
	case len(word) >= minStemLength && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") && !strings.HasSuffix(word, "us"):
		return word[:len(word)-1]
	}
	return word
}

func hasSibilantPlural(word string) bool {
	switch {
	case strings.HasSuffix(word, "xes"), strings.HasSuffix(word, "zes"), strings.HasSuffix(word, "ches"), strings.HasSuffix(word, "shes"):
		return true
	case strings.HasSuffix(word, "sses"), strings.HasSuffix(word, "tuses"):
		return true
	}
	return false
}

func undouble(stem string) string {
	n := len(stem)
	if n >= 2 && stem[n-1] == stem[n-2] && strings.IndexByte("bgmnprt", stem[n-1]) >= 0 {
		return stem[:n-1]
	}
	return stem
}

func queryTerms(query string) []string {
	var terms []string
	for _, w := range words(query) {
		if !stopWords[w] && !slices.Contains(terms, w) {
			terms = append(terms, w)
		}
	}
	return terms
}

func parameterNames(schema json.RawMessage) []string {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(schema, &parsed) != nil {
		return nil
	}
	return slices.Sorted(maps.Keys(parsed.Properties))
}

func searchableOf(source Source, tool Tool) searchable {
	s := searchable{source: source, tool: tool, name: words(tool.Name), words: map[string]bool{}}
	s.whole = strings.Join(s.name, " ")
	for _, name := range parameterNames(tool.Pinned.InputSchema) {
		s.params = append(s.params, words(name)...)
	}
	for _, w := range words(tool.Pinned.Description) {
		s.words[w] = true
	}
	return s
}

func (s searchable) mentions(term string) bool {
	return slices.Contains(s.name, term) || slices.Contains(s.params, term) || s.words[term]
}

func (s searchable) score(terms []string, weight map[string]float64) float64 {
	var total float64
	for _, term := range terms {
		var best float64
		switch {
		case slices.Contains(s.name, term):
			best = scoreNameWord
		case len(term) >= minPrefixLength && slices.ContainsFunc(s.name, func(w string) bool { return strings.HasPrefix(w, term) }):
			best = scoreNamePrefix
		}
		if slices.Contains(s.params, term) {
			best = max(best, scoreParameter)
		}
		if s.words[term] {
			best = max(best, scoreDescription)
		}
		total += best * weight[term]
	}
	if len(terms) > 1 && strings.Contains(s.whole, strings.Join(terms, " ")) {
		total += scoreWholeName
	}
	return total
}

func rank(query string, candidates []searchable) []scored {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil
	}
	weight := map[string]float64{}
	for _, term := range terms {
		df := 0
		for _, c := range candidates {
			if c.mentions(term) || slices.ContainsFunc(c.name, func(w string) bool { return len(term) >= minPrefixLength && strings.HasPrefix(w, term) }) {
				df++
			}
		}
		weight[term] = math.Log(1 + float64(len(candidates))/float64(max(df, 1)))
	}
	var out []scored
	for _, c := range candidates {
		if score := c.score(terms, weight); score > 0 {
			out = append(out, scored{searchable: c, score: score})
		}
	}
	slices.SortFunc(out, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), strings.Compare(a.tool.Name, b.tool.Name), strings.Compare(a.source.UID, b.source.UID))
	})
	return out
}
