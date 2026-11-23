package jaeger

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	keyService     = "service"
	keyOperation   = "operation"
	keyMinDuration = "minDuration"
	keyMaxDuration = "maxDuration"
)

type search struct {
	service, operation       string
	tags                     map[string]string
	minDuration, maxDuration time.Duration
}

type problem struct {
	message    string
	start, end int
}

func (p problem) err() error {
	return datasource.Errorf(datasource.ErrInvalidQuery, "invalid Jaeger search at offset %d: %s", p.start, p.message)
}

type pair struct {
	key, value             string
	start, valueStart, end int
}

func tokenize(text string) ([]pair, *problem) {
	var pairs []pair
	for i := 0; i < len(text); {
		if isSpace(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && !isSpace(text[i]) && text[i] != '=' && text[i] != '"' {
			i++
		}
		if i >= len(text) || text[i] != '=' || i == start {
			for i < len(text) && !isSpace(text[i]) {
				i++
			}
			return nil, &problem{"expected key=value, as in service=checkout", start, i}
		}
		key := text[start:i]
		i++
		valueStart := i
		value, end, ok := valueAt(text, i)
		if !ok {
			return nil, &problem{"unterminated quoted value", i, len(text)}
		}
		i = end
		if value == "" {
			return nil, &problem{key + " needs a value", start, i}
		}
		pairs = append(pairs, pair{key: key, value: value, start: start, valueStart: valueStart, end: i})
	}
	return pairs, nil
}

func valueAt(text string, i int) (string, int, bool) {
	if i < len(text) && text[i] == '"' {
		return quoted(text, i)
	}
	start := i
	for i < len(text) && !isSpace(text[i]) {
		i++
	}
	return text[start:i], i, true
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func quoted(text string, open int) (string, int, bool) {
	var b strings.Builder
	for i := open + 1; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\\' && i+1 < len(text):
			i++
			b.WriteByte(text[i])
		case c == '"':
			return b.String(), i + 1, true
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, false
}

func parseSearch(text string) (search, *problem) {
	pairs, bad := tokenize(text)
	if bad != nil {
		return search{}, bad
	}
	s := search{tags: map[string]string{}}
	seen := map[string]bool{}
	var needsService *pair
	for _, p := range pairs {
		if seen[p.key] {
			return search{}, &problem{p.key + " is given twice", p.start, p.end}
		}
		seen[p.key] = true
		switch p.key {
		case keyService:
			s.service = p.value
		case keyOperation:
			s.operation = p.value
			if needsService == nil {
				needsService = &p
			}
		case keyMinDuration, keyMaxDuration:
			d, err := time.ParseDuration(p.value)
			if err != nil || d < 0 {
				return search{}, &problem{p.key + ` must be a duration such as 500ms or 2s`, p.valueStart, p.end}
			}
			if p.key == keyMinDuration {
				s.minDuration = d
			} else {
				s.maxDuration = d
			}
		default:
			if !utf8.ValidString(p.value) {
				return search{}, &problem{p.key + " is not valid UTF-8", p.valueStart, p.end}
			}
			s.tags[p.key] = p.value
			if needsService == nil {
				needsService = &p
			}
		}
	}
	if s.service == "" && needsService != nil {
		what := "tags need"
		if needsService.key == keyOperation {
			what = keyOperation + " needs"
		}
		return search{}, &problem{what + " " + keyService + "=<name>", needsService.start, needsService.end}
	}
	if s.maxDuration > 0 && s.minDuration > s.maxDuration {
		return search{}, &problem{keyMinDuration + " is above " + keyMaxDuration, 0, len(text)}
	}
	return s, nil
}

func (s search) attributesJSON() (string, error) {
	b, err := json.Marshal(s.tags)
	return string(b), err
}
