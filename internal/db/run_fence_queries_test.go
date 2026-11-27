package db_test

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

var holderWrites = []string{
	"RenewAgentRequestLease", "AppendAgentRequestStep",
	"UpsertAgentRequestBuild", "RebaseAgentRequestBuild", "DeleteAgentRequestBuild",
	"SucceedAgentRequest", "SucceedSuggestRequest", "SucceedFindingRequest", "SucceedProposalRequest",
	"FailAgentRequest", "SettleAgentRequest", "ReleaseAgentRequest",
}

var (
	queryHeader   = regexp.MustCompile(`(?m)^-- name: (\w+) :\w+$`)
	comparedToArg = regexp.MustCompile(`(?:^|[^:\w.])(?:\w+\.)?(\w+) = @\w+`)
)

func namedQueries(t *testing.T) map[string]string {
	t.Helper()
	dir := os.DirFS("queries")
	files, err := fs.Glob(dir, "*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("list the query files: %v, found %d", err, len(files))
	}
	queries := map[string]string{}
	for _, file := range files {
		raw, err := fs.ReadFile(dir, file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		headers := queryHeader.FindAllStringSubmatchIndex(text, -1)
		for i, h := range headers {
			end := len(text)
			if i+1 < len(headers) {
				end = headers[i+1][0]
			}
			queries[text[h[2]:h[3]]] = text[h[1]:end]
		}
	}
	return queries
}

func columnsComparedToAnArgument(query string) []string {
	var columns []string
	for _, m := range comparedToArg.FindAllStringSubmatch(query, -1) {
		columns = append(columns, m[1])
	}
	return columns
}

func lowers(query, column string) bool {
	return strings.Contains(strings.Join(strings.Fields(query), " "), column+" = "+column+" - ")
}

func TestNoQueryLowersAColumnAHolderWriteIsFencedOn(t *testing.T) {
	queries := namedQueries(t)
	fencedBy := map[string]string{}
	for _, name := range holderWrites {
		query, found := queries[name]
		if !found {
			t.Fatalf("query %s is gone; name the query that replaced it in holderWrites", name)
		}
		for _, column := range columnsComparedToAnArgument(query) {
			fencedBy[column] = name
		}
	}
	if len(fencedBy) == 0 {
		t.Fatal("no holder write compares a column to an argument")
	}
	for name, query := range queries {
		for column, write := range fencedBy {
			if lowers(query, column) {
				t.Errorf("%s lowers %s, which %s compares to what its holder was handed: the next claim hands the same number to a new holder", name, column, write)
			}
		}
	}
}
