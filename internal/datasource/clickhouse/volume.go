package clickhouse

import (
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

const (
	volumeTimeColumn  = "Timestamp"
	volumeLevelColumn = "SeverityText"
	volumeNeedsSelect = "the volume needs one SELECT without UNION, INTERSECT, EXCEPT or LIMIT BY"
)

var volumeSetOperators = map[string]bool{"UNION": true, "INTERSECT": true, "EXCEPT": true}

func VolumeQuery(query string, step time.Duration) (string, error) {
	inner, err := volumeInner(query)
	if err != nil {
		return "", err
	}
	seconds := max(int64(step/time.Second), 1)
	return "SELECT toStartOfInterval(" + quoteIdent(volumeTimeColumn) + ", INTERVAL " + strconv.FormatInt(seconds, 10) + " SECOND) AS Time, " +
		volumeLevelFold() + " AS Level, count() AS Count\nFROM (\n" + inner + "\n)\nGROUP BY Time, Level\nORDER BY Time", nil
}

func volumeInner(query string) (string, error) {
	tokens, err := macros.TokenizeClickHouse(query)
	if err != nil {
		return "", err
	}
	sig := significant(tokens)
	end := len(query)
	if n := len(sig); n > 0 && isPunct(query, sig[n-1], ";") {
		end = sig[n-1].Start
	}
	cut, depth := volumeCut{at: -1}, 0
	for i, t := range sig {
		switch {
		case isPunct(query, t, "("):
			depth++
		case isPunct(query, t, ")"):
			depth--
		case depth == 0 && isClauseWord(query, sig, i):
			if err := cut.observe(query, sig, i); err != nil {
				return "", err
			}
		}
	}
	if cut.at >= 0 {
		end = cut.at
	}
	return query[:end], nil
}

type volumeCut struct {
	at      int
	limited bool
}

func (c *volumeCut) observe(query string, sig []macros.Token, i int) error {
	word := strings.ToUpper(query[sig[i].Start:sig[i].End])
	switch {
	case volumeSetOperators[word], word == "BY" && c.limited:
		return datasource.Errorf(datasource.ErrInvalidQuery, "%s", volumeNeedsSelect)
	case word == "LIMIT":
		c.limited = true
		c.mark(sig[i].Start)
	case word == "ORDER" && i+1 < len(sig) && strings.EqualFold(query[sig[i+1].Start:sig[i+1].End], "BY"):
		c.mark(sig[i].Start)
	}
	return nil
}

func (c *volumeCut) mark(at int) {
	if c.at < 0 {
		c.at = at
	}
}

func isClauseWord(query string, sig []macros.Token, i int) bool {
	return sig[i].Kind == macros.TokenWord && (i == 0 || !isPunct(query, sig[i-1], "."))
}

func volumeLevelFold() string {
	level := "lower(trimBoth(toString(" + quoteIdent(volumeLevelColumn) + ")))"
	var b strings.Builder
	b.WriteString("multiIf(")
	for _, kind := range logvolume.Kinds {
		words := logvolume.Aliases(kind)
		if len(words) == 0 {
			continue
		}
		b.WriteString(level + " IN ('" + strings.Join(words, "', '") + "'), '" + string(kind) + "', ")
	}
	b.WriteString("'" + string(logvolume.Other) + "')")
	return b.String()
}
