package tools

import "regexp"

var rejectionReasons = []struct {
	match  *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`^unknown field`), "it used an option the tool does not take"},
	{regexp.MustCompile(`valid JSON|single JSON object|must be a JSON object`), "its input was not valid JSON"},
	{regexp.MustCompile(`wrong type|must be an integer|must be a string|must be a positive integer|must be \w+, not `), "a value had the wrong type"},
	{regexp.MustCompile(`\b(?:is|are) required|^\w+ needs `), "a required value was missing"},
	{regexp.MustCompile(`time range|RFC 3339|before from|from and to|^to: |duration|clear_range|^window`), "its time range was not valid"},
	{regexp.MustCompile(`at most \d* ?(?:bytes|characters|days)|takes at most`), "a value was too long"},
	{regexp.MustCompile(`UUID|not valid|hex characters|must be one of|must be at least|must be between|must match|^\w+ must be \w+`), "a value was not in the allowed form"},
	{regexp.MustCompile(`^this run `), "that is not available in this kind of run"},
}

func SentBackStep(tool, reason string) string {
	step := "The " + tool + " call was sent back for a fix"
	if reason == "" {
		return step
	}
	return step + ": " + reason
}

func rejectionReason(eb errorBody) string {
	if eb.unknownField {
		return rejectionReasons[0].reason
	}
	for _, r := range rejectionReasons {
		if r.match.MatchString(eb.Message) {
			return r.reason
		}
	}
	return ""
}
