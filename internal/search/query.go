package search

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxQueryRunes  = 200
	minPrefixRunes = 2
)

type InvalidQueryError struct {
	Message string
}

func (e *InvalidQueryError) Error() string { return e.Message }

type Query struct {
	Words  string
	Prefix string
}

func ParseQuery(raw string) (Query, error) {
	words := splitHyphenatedWords(strings.TrimSpace(raw))
	if n := utf8.RuneCountInString(words); n == 0 || n > MaxQueryRunes {
		return Query{}, &InvalidQueryError{Message: "q must be 1 to 200 characters"}
	}
	for _, r := range words {
		if unicode.IsControl(r) {
			return Query{}, &InvalidQueryError{Message: "q must not contain control characters"}
		}
	}
	if !hasSearchableWord(words) {
		return Query{}, &InvalidQueryError{Message: "q needs a word to search for"}
	}
	return Query{Words: words, Prefix: trailingPrefix(raw, words)}, nil
}

func splitHyphenatedWords(words string) string {
	var out strings.Builder
	runes := []rune(words)
	inQuote := false
	for start := 0; start < len(runes); {
		if unicode.IsSpace(runes[start]) {
			out.WriteRune(runes[start])
			start++
			continue
		}
		end := start
		for end < len(runes) && !unicode.IsSpace(runes[end]) {
			end++
		}
		token := runes[start:end]
		excluded := !inQuote && token[0] == '-'
		split := splitToken(token)
		if excluded && split.changed && !strings.ContainsRune(split.text, '"') {
			out.WriteString("-\"")
			out.WriteString(split.text[1:])
			out.WriteString("\"")
		} else {
			out.WriteString(split.text)
		}
		inQuote = inQuote != (strings.Count(split.text, "\"")%2 == 1)
		start = end
	}
	return out.String()
}

type splitResult struct {
	text    string
	changed bool
}

func splitToken(token []rune) splitResult {
	parts := make([]rune, len(token))
	copy(parts, token)
	changed := false
	for i, r := range parts {
		if r == '-' && i > 0 && i+1 < len(parts) && isWordRune(parts[i-1]) && unicode.IsLetter(parts[i+1]) {
			parts[i] = ' '
			changed = true
		}
	}
	return splitResult{text: string(parts), changed: changed}
}

func hasSearchableWord(words string) bool {
	for _, token := range strings.Fields(words) {
		if strings.HasPrefix(token, "-") {
			continue
		}
		for _, r := range token {
			if isWordRune(r) {
				return true
			}
		}
	}
	return false
}

func trailingPrefix(raw, words string) string {
	last, _ := utf8.DecodeLastRuneInString(raw)
	if unicode.IsSpace(last) || strings.Count(words, `"`)%2 != 0 {
		return ""
	}
	fields := strings.Fields(words)
	if strings.HasPrefix(fields[len(fields)-1], "-") {
		return ""
	}
	end := len(words)
	start := end
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(words[:start])
		if !isWordRune(r) {
			break
		}
		start -= size
	}
	if utf8.RuneCountInString(words[start:end]) < minPrefixRunes {
		return ""
	}
	return words[start:end]
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}
