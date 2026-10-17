package clickhouse

import (
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

var forbiddenClauses = map[string]bool{"SETTINGS": true, "FORMAT": true, "INTO": true, "OUTFILE": true}

var forbiddenFunctions = map[string]bool{
	"url": true, "urlcluster": true, "file": true, "filecluster": true,
	"remote": true, "remotesecure": true, "cluster": true, "clusterallreplicas": true,
	"s3": true, "s3cluster": true, "oss": true, "gcs": true, "azureblobstorage": true, "azureblobstoragecluster": true,
	"hdfs": true, "hdfscluster": true, "mysql": true, "postgresql": true, "mongodb": true,
	"redis": true, "sqlite": true, "jdbc": true, "odbc": true, "executable": true, "input": true,
	"arrowflight": true, "ytsaurus": true, "hive": true,
	"cosn": true, "bigquery": true, "filesystem": true, "eval": true,
}

var allowedTableFunctions = map[string]bool{
	"numbers": true, "numbers_mt": true, "zeros": true, "zeros_mt": true,
	"generateseries": true, "generate_series": true, "values": true, "null": true,
	"view": true, "primes": true, "generaterandom": true,
}

var fallbackDualUse = map[string]bool{"format": true, "fuzzquery": true}

var fromEndingKeywords = map[string]bool{
	"SELECT": true, "WITH": true, "WHERE": true, "PREWHERE": true, "GROUP": true, "HAVING": true,
	"ORDER": true, "LIMIT": true, "OFFSET": true, "WINDOW": true, "QUALIFY": true,
	"UNION": true, "INTERSECT": true, "EXCEPT": true, "SETTINGS": true, "FORMAT": true,
}

type tableFunctionSet struct {
	denied map[string]bool
	dual   map[string]bool
}

func newTableFunctionSet(names []string, alsoScalar map[string]bool) *tableFunctionSet {
	set := &tableFunctionSet{denied: map[string]bool{}, dual: map[string]bool{}}
	for _, name := range names {
		name = strings.ToLower(name)
		switch {
		case allowedTableFunctions[name]:
		case alsoScalar[name] || fallbackDualUse[name]:
			set.dual[name] = true
		default:
			set.denied[name] = true
		}
	}
	return set
}

var forbiddenFunctionPrefixes = []string{"iceberg", "deltalake", "hudi", "paimon"}

func guard(query string) (string, error) { return guardWith(query, nil) }

func (d *Datasource) guard(query string) (string, error) {
	return guardWith(query, d.tableFunctions.Load())
}

func guardWith(query string, tf *tableFunctionSet) (string, error) {
	tokens, err := macros.TokenizeClickHouse(query)
	if err != nil {
		return "", err
	}
	sig := significant(tokens)
	if len(sig) == 0 {
		return "", forbidden("empty query")
	}
	if !startsStatement(query, sig[0]) {
		return "", forbidden("only a single SELECT statement is allowed")
	}
	end := len(query)
	for i, t := range sig {
		if isPunct(query, t, "$") {
			return "", datasource.Errorf(datasource.ErrInvalidQuery, "unterminated literal")
		}
		if isPunct(query, t, ";") {
			if i != len(sig)-1 {
				return "", forbidden("multiple statements are not allowed")
			}
			end = t.Start
		}
	}
	if err := checkTokens(query, sig, tf); err != nil {
		return "", err
	}
	return query[:end], nil
}

func significant(tokens []macros.Token) []macros.Token {
	sig := make([]macros.Token, 0, len(tokens))
	for _, t := range tokens {
		if t.Kind != macros.TokenWhitespace && t.Kind != macros.TokenComment {
			sig = append(sig, t)
		}
	}
	return sig
}

func startsStatement(query string, t macros.Token) bool {
	if t.Kind != macros.TokenWord {
		return false
	}
	word := query[t.Start:t.End]
	return strings.EqualFold(word, "SELECT") || strings.EqualFold(word, "WITH")
}

func isPunct(query string, t macros.Token, text string) bool {
	return t.Kind == macros.TokenPunct && query[t.Start:t.End] == text
}

type fromTracker struct{ inFrom []bool }

func newFromTracker() *fromTracker { return &fromTracker{inFrom: []bool{false}} }

func (f *fromTracker) top() bool { return f.inFrom[len(f.inFrom)-1] }

func (f *fromTracker) set(v bool) { f.inFrom[len(f.inFrom)-1] = v }

func (f *fromTracker) observe(query string, t macros.Token, calls, afterDot bool) {
	switch {
	case t.Kind == macros.TokenPunct && query[t.Start:t.End] == "(":
		f.inFrom = append(f.inFrom, false)
	case t.Kind == macros.TokenPunct && query[t.Start:t.End] == ")":
		if len(f.inFrom) > 1 {
			f.inFrom = f.inFrom[:len(f.inFrom)-1]
		}
	case t.Kind == macros.TokenWord && !afterDot:
		upper := strings.ToUpper(query[t.Start:t.End])
		if calls && forbiddenClauses[upper] {
			return
		}
		switch {
		case upper == "FROM" || upper == "JOIN":
			f.set(true)
		case fromEndingKeywords[upper]:
			f.set(false)
		}
	}
}

func checkTokens(query string, sig []macros.Token, tf *tableFunctionSet) error {
	from := newFromTracker()
	for i, t := range sig {
		calls := i+1 < len(sig) && isPunct(query, sig[i+1], "(")
		afterDot := i > 0 && isPunct(query, sig[i-1], ".")
		if t.Kind == macros.TokenWord && !calls && !afterDot {
			if upper := strings.ToUpper(query[t.Start:t.End]); forbiddenClauses[upper] {
				return forbidden("%s clauses are not allowed", upper)
			}
		}
		if calls {
			if err := checkCall(query, t, tf, from.top()); err != nil {
				return err
			}
		}
		from.observe(query, t, calls, afterDot)
	}
	return nil
}

func checkCall(query string, t macros.Token, tf *tableFunctionSet, inFrom bool) error {
	switch t.Kind {
	case macros.TokenQuotedIdent:
		if !t.NameExact {
			return forbidden("quoted function names with unsupported escapes are not allowed")
		}
	case macros.TokenWord:
	default:
		return nil
	}
	if name := strings.ToLower(identName(query, t)); isForbiddenFunction(name, tf, inFrom) {
		return forbidden("function %s() is not allowed", name)
	}
	return nil
}

// A name that is both a scalar and a table function (such as format) is refused only inside a
// FROM clause, found by tracking FROM/JOIN per parenthesis depth; any name that is only a table
// function is refused wherever it is called.
func isForbiddenFunction(name string, tf *tableFunctionSet, inFrom bool) bool {
	if allowedTableFunctions[name] {
		return false
	}
	if forbiddenFunctions[name] {
		return true
	}
	for _, prefix := range forbiddenFunctionPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	if tf != nil && tf.denied[name] {
		return true
	}
	return inFrom && (fallbackDualUse[name] || (tf != nil && tf.dual[name]))
}

func identName(query string, t macros.Token) string {
	if t.Kind == macros.TokenQuotedIdent {
		return t.Name
	}
	return query[t.Start:t.End]
}

func forbidden(format string, args ...any) error {
	return datasource.Errorf(datasource.ErrForbiddenQuery, format, args...)
}
