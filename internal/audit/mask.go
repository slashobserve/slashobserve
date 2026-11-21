package audit

import (
	"regexp"
	"slices"
	"strings"
)

const (
	maskedSecret   = "***"
	maskSlackBytes = 4096
	maxCallBytes   = 2048
	maxCallArgs    = 16
)

type secretPattern struct {
	re   *regexp.Regexp
	from int
}

var secretPatterns = []secretPattern{
	{regexp.MustCompile("([A-Za-z][A-Za-z0-9+.-]*://)([^\\s/'\"`]*)@"), 2},
	{regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|secret|token|api_key|apikey|access_key|secret_key|authorization)["']?\s*[:=]\s*(?:bearer\s+)?(?:"([^"]*)"|'([^']*)'|([^\s,;&)}\]"'~=!][^\s,;&)}\]"']*))`), 1},
	{regexp.MustCompile(`(?i)\b(bearer\s+)([A-Za-z0-9._~+/=-]{16,})`), 2},
	{regexp.MustCompile(`\b((?:AKIA|ASIA)[0-9A-Z]{16})\b`), 1},
	{regexp.MustCompile(`\b(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)`), 1},
	{regexp.MustCompile(`(?s)(-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$))`), 1},
}

var credentialFunction = regexp.MustCompile(`(?i)\b(s3cluster|s3|gcs|azureblobstorage|hdfs|url|mysql|postgresql|mongodb|redis|remotesecure|remote|jdbc|odbc)\s*\(`)

type argumentRule struct {
	positions []int
	whole     bool
}

var credentialArguments = map[string]argumentRule{
	"s3":               {positions: []int{1, 2, 3}},
	"s3cluster":        {positions: []int{2, 3, 4}},
	"gcs":              {positions: []int{1, 2}},
	"azureblobstorage": {positions: []int{4}},
	"mysql":            {positions: []int{4}},
	"postgresql":       {positions: []int{4}},
	"mongodb":          {positions: []int{4}},
	"redis":            {positions: []int{4}},
	"remote":           {positions: []int{3, 4}},
	"remotesecure":     {positions: []int{3, 4}},
	"jdbc":             {positions: []int{0}, whole: true},
	"odbc":             {positions: []int{0}, whole: true},
}

var namedCredentials = []string{
	"access_key_id", "secret_access_key", "session_token", "aws_access_key_id", "aws_secret_access_key",
	"password", "account_key", "connection_string", "hmac_key", "hmac_secret",
}

var plainOption = regexp.MustCompile(`(?i)^(auto|nosign|none|gzip|gz|brotli|br|xz|zstd|zst|lz4|bz2|deflate|snappy|csv.*|tsv.*|tabseparated.*|json.*|ndjson|parquet|orc|avro.*|arrow.*|native|rawblob|lineasstring|values|protobuf.*|msgpack|template|regexp|customseparated.*|markdown)$`)

var headerValue = regexp.MustCompile(`=\s*(?:'([^']*)'|"([^"]*)")`)

var headersCall = regexp.MustCompile(`(?i)^headers\s*\(`)

func maskSecrets(text string) string {
	text = applyEdits(text, tableFunctionEdits(text))
	for _, p := range secretPatterns {
		text = applyEdits(text, p.edits(text))
	}
	return text
}

type edit struct{ start, end int }

func (p secretPattern) edits(text string) []edit {
	var edits []edit
	for _, m := range p.re.FindAllStringSubmatchIndex(text, -1) {
		for g := p.from; 2*g+1 < len(m); g++ {
			if m[2*g] >= 0 && m[2*g+1] > m[2*g] {
				edits = append(edits, edit{m[2*g], m[2*g+1]})
			}
		}
	}
	return edits
}

func applyEdits(text string, edits []edit) string {
	if len(edits) == 0 {
		return text
	}
	slices.SortFunc(edits, func(a, b edit) int { return a.start - b.start })
	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for _, e := range edits {
		if e.start < last {
			continue
		}
		b.WriteString(text[last:e.start])
		b.WriteString(starsFor(e.end - e.start))
		last = e.end
	}
	b.WriteString(text[last:])
	return b.String()
}

func starsFor(n int) string {
	return maskedSecret[:min(n, len(maskedSecret))]
}

type span struct{ start, end int }

func tableFunctionEdits(text string) []edit {
	var edits []edit
	for _, m := range credentialFunction.FindAllStringSubmatchIndex(text, -1) {
		name := strings.ToLower(text[m[2]:m[3]])
		rule := credentialArguments[name]
		for i, arg := range callArguments(text, m[1]) {
			switch {
			case slices.Contains(rule.positions, i):
				edits = append(edits, literalEdit(text, arg, !rule.whole)...)
			case name == "azureblobstorage" && i == 0 && connectionString(text[arg.start:arg.end]):
				edits = append(edits, literalEdit(text, arg, false)...)
			}
			edits = append(edits, namedCredentialEdit(text, arg)...)
			edits = append(edits, headerEdits(text, arg)...)
		}
	}
	return edits
}

func connectionString(arg string) bool {
	lower := strings.ToLower(arg)
	return strings.Contains(lower, "accountkey") || strings.Contains(lower, "sharedaccesssignature") || strings.Contains(lower, "sig=")
}

func literalEdit(text string, arg span, skipOptions bool) []edit {
	inner, ok := literalInner(text, arg)
	if !ok || inner.end <= inner.start || (skipOptions && isOption(text[inner.start:inner.end])) {
		return nil
	}
	return []edit{{inner.start, inner.end}}
}

func literalInner(text string, arg span) (span, bool) {
	if arg.end-arg.start < 1 {
		return span{}, false
	}
	q := text[arg.start]
	if q != '\'' && q != '"' {
		return span{}, false
	}
	if arg.end-arg.start >= 2 && text[arg.end-1] == q {
		return span{arg.start + 1, arg.end - 1}, true
	}
	return span{arg.start + 1, arg.end}, true
}

func isOption(v string) bool {
	return plainOption.MatchString(v) || strings.ContainsAny(v, " \t\r\n")
}

func namedCredentialEdit(text string, arg span) []edit {
	name, _, found := strings.Cut(text[arg.start:arg.end], "=")
	if !found || !slices.Contains(namedCredentials, strings.ToLower(strings.TrimSpace(name))) {
		return nil
	}
	valueStart := arg.start + len(name) + 1
	for valueStart < arg.end && strings.ContainsRune(" \t\r\n", rune(text[valueStart])) {
		valueStart++
	}
	return literalEdit(text, span{valueStart, arg.end}, false)
}

func headerEdits(text string, arg span) []edit {
	if !headersCall.MatchString(text[arg.start:arg.end]) {
		return nil
	}
	var edits []edit
	for _, m := range headerValue.FindAllStringSubmatchIndex(text[arg.start:arg.end], -1) {
		for g := 1; g <= 2; g++ {
			if m[2*g] >= 0 && m[2*g+1] > m[2*g] {
				edits = append(edits, edit{arg.start + m[2*g], arg.start + m[2*g+1]})
			}
		}
	}
	return edits
}

func callArguments(text string, open int) []span {
	limit := min(len(text), open+maxCallBytes)
	var args []span
	push := func(start, end int) { args = append(args, trimmed(text, start, end)) }
	start, depth := open, 0
	var quote byte
	for i := open; i < limit && len(args) < maxCallArgs; i++ {
		c := text[i]
		if quote != 0 {
			i, quote = inQuote(text, i, limit, quote)
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			} else if c == ')' {
				push(start, i)
				return args
			}
		case ',':
			if depth == 0 {
				push(start, i)
				start = i + 1
			}
		}
	}
	if len(args) < maxCallArgs {
		push(start, limit)
	}
	return args
}

func inQuote(text string, i, limit int, quote byte) (int, byte) {
	switch {
	case text[i] == '\\':
		return i + 1, quote
	case text[i] != quote:
		return i, quote
	case i+1 < limit && text[i+1] == quote:
		return i + 1, quote
	}
	return i, 0
}

func trimmed(text string, start, end int) span {
	for start < end && strings.ContainsRune(" \t\r\n", rune(text[start])) {
		start++
	}
	for end > start && strings.ContainsRune(" \t\r\n", rune(text[end-1])) {
		end--
	}
	return span{start, end}
}
