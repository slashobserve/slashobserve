package audit

import (
	"regexp"
	"slices"
	"strings"
)

const (
	maskedSecret   = "***"
	maskSlackBytes = 4096
)

const (
	assignment = `["']?\s*(?:!=|=~|!~|[:=])\s*`
	scheme     = `(?:(?:bearer|basic|token|digest|negotiate|ntlm|apikey|jwt|oauth|hmac|mac)\s+)?`
	secretName = `(?:password|passwd|pwd|secret[_-]?access[_-]?key|secret[_-]?key|secret|token|api[_-]?key|access[_-]?key)`
)

var secretNameRe = regexp.MustCompile(`(?i)` + secretName)

func SecretName(name string) bool {
	return secretNameRe.MatchString(name)
}

func valueOf(prefix, bare string) string {
	return `(?:"` + prefix + `((?:[^"\\]|\\.)*)"|'` + prefix + `((?:[^'\\]|\\.|'')*)'|"` + prefix + `((?:[^"\\]|\\.)*)\\?$|'` + prefix + `((?:[^'\\]|\\.|'')*)\\?$|` + prefix + bare + `)`
}

type secretPattern struct {
	re   *regexp.Regexp
	from int
}

var secretPatterns = []secretPattern{
	{regexp.MustCompile("([A-Za-z][A-Za-z0-9+.-]*://)([^\\s/'\"`]*)@"), 2},
	{regexp.MustCompile(`(?i)` + secretName + assignment + `(?:bearer\s+)?` + valueOf("", `([^\s,;&)}\]"']+)`)), 1},
	{regexp.MustCompile(`(?i)authorization` + assignment + valueOf(scheme, `([^\r\n"']+)`)), 1},
	{regexp.MustCompile(`(?i)\b(bearer\s+)([A-Za-z0-9._~+/=-]{16,})`), 2},
	{regexp.MustCompile(`\b((?:AKIA|ASIA)[0-9A-Z]{16})\b`), 1},
	{regexp.MustCompile(`\b(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)`), 1},
	{regexp.MustCompile(`(?s)(-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$))`), 1},
}

var credentialFunction = regexp.MustCompile(`(?i)\b(materializedpostgresql|materializedmysql|azureblobstoragecluster|azureblobstorage|s3cluster|s3queue|s3|gcs|deltalake|iceberg[a-z0-9]*|hudi|hdfs|url|mysql|postgresql|mongodb|redis|remotesecure|remote|jdbc|odbc)\s*\(`)

type argumentRule struct {
	positions  []int
	whole      bool
	connection []int
	shortLen   int
	whenShort  []int
}

var objectStore = argumentRule{positions: []int{1, 2, 3}}

var credentialArguments = map[string]argumentRule{
	"s3":                      objectStore,
	"s3queue":                 objectStore,
	"deltalake":               objectStore,
	"iceberg":                 objectStore,
	"hudi":                    objectStore,
	"s3cluster":               {positions: []int{2, 3, 4}},
	"gcs":                     {positions: []int{1, 2}},
	"azureblobstorage":        {positions: []int{4}, connection: []int{0}},
	"azureblobstoragecluster": {positions: []int{5}, connection: []int{1}},
	"mysql":                   {positions: []int{4}, shortLen: 4, whenShort: []int{3}},
	"postgresql":              {positions: []int{4}, shortLen: 4, whenShort: []int{3}},
	"mongodb":                 {positions: []int{4}},
	"redis":                   {positions: []int{4}},
	"remote":                  {positions: []int{3, 4}},
	"remotesecure":            {positions: []int{3, 4}},
	"materializedpostgresql":  {positions: []int{4}, shortLen: 4, whenShort: []int{3}},
	"materializedmysql":       {positions: []int{3}},
	"jdbc":                    {positions: []int{0}, whole: true},
	"odbc":                    {positions: []int{0}, whole: true},
}

func ruleFor(name string) argumentRule {
	if strings.HasPrefix(name, "iceberg") {
		name = "iceberg"
	}
	return credentialArguments[name]
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
		rule := ruleFor(strings.ToLower(text[m[2]:m[3]]))
		args := callArguments(text, m[1])
		for i, arg := range args {
			switch {
			case slices.Contains(rule.positions, i) || (len(args) <= rule.shortLen && slices.Contains(rule.whenShort, i)):
				edits = append(edits, literalEdit(text, arg, !rule.whole)...)
			case slices.Contains(rule.connection, i) && connectionString(text[arg.start:arg.end]):
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
	if strings.HasPrefix(text[arg.start:arg.end], "$$") {
		if arg.end-arg.start >= 4 && strings.HasSuffix(text[arg.start:arg.end], "$$") {
			return span{arg.start + 2, arg.end - 2}, true
		}
		return span{arg.start + 2, arg.end}, true
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

type argScanner struct {
	args        []span
	first, last int
}

func (s *argScanner) see(i int) {
	if s.first < 0 {
		s.first = i
	}
	s.last = i + 1
}

func (s *argScanner) push() {
	if s.first < 0 {
		s.args = append(s.args, span{})
	} else {
		s.args = append(s.args, span{s.first, s.last})
	}
	s.first, s.last = -1, -1
}

func callArguments(text string, open int) []span {
	s := &argScanner{first: -1, last: -1}
	depth := 0
	var quote byte
	for i := open; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			i, quote = inQuote(text, i, len(text), quote)
			s.see(min(i, len(text)-1))
			continue
		}
		if end := commentEnd(text, i); end > i {
			i = end - 1
			continue
		}
		if end := heredocEnd(text, i); end > i {
			s.see(i)
			s.see(end - 1)
			i = end - 1
			continue
		}
		switch tokenOf(c, depth) {
		case tokSpace:
			continue
		case tokEnd:
			s.push()
			return s.args
		case tokComma:
			s.push()
			continue
		case tokQuote:
			quote = c
		case tokOpen:
			depth++
		case tokClose:
			depth--
		}
		s.see(i)
	}
	s.push()
	return s.args
}

type token int

const (
	tokOther token = iota
	tokSpace
	tokEnd
	tokComma
	tokQuote
	tokOpen
	tokClose
)

func tokenOf(c byte, depth int) token {
	switch c {
	case ' ', '\t', '\r', '\n':
		return tokSpace
	case '\'', '"', '`':
		return tokQuote
	case '(', '[', '{':
		return tokOpen
	case ')':
		if depth == 0 {
			return tokEnd
		}
		return tokClose
	case ']', '}':
		if depth > 0 {
			return tokClose
		}
	case ',':
		if depth == 0 {
			return tokComma
		}
	}
	return tokOther
}

func heredocEnd(text string, i int) int {
	if !strings.HasPrefix(text[i:], "$$") {
		return i
	}
	if end := strings.Index(text[i+2:], "$$"); end >= 0 {
		return i + 2 + end + 2
	}
	return len(text)
}

func commentEnd(text string, i int) int {
	switch {
	case strings.HasPrefix(text[i:], "--"), text[i] == '#':
		if nl := strings.IndexByte(text[i:], '\n'); nl >= 0 {
			return i + nl
		}
		return len(text)
	case strings.HasPrefix(text[i:], "/*"):
		if end := strings.Index(text[i+2:], "*/"); end >= 0 {
			return i + 2 + end + 2
		}
		return len(text)
	}
	return i
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
