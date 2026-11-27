package db_test

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

const claimQuery = "ClaimAgentRequest"

var holderQueries = []string{
	"RenewAgentRequestLease", "AppendAgentRequestStep",
	"UpsertAgentRequestBuild", "RebaseAgentRequestBuild", "DeleteAgentRequestBuild",
	"SucceedAgentRequest", "SucceedSuggestRequest", "SucceedFindingRequest", "SucceedProposalRequest",
	"FailAgentRequest", "SettleAgentRequest", "ReleaseAgentRequest",
	"GetClaimedAgentRequest", "GetRunTokenDeadline", "RenewRunTokens",
}

var (
	queryHeader        = regexp.MustCompile(`(?m)^-- name: (\w+) :\w+$`)
	fencedOnClaimEpoch = regexp.MustCompile(`\bclaim_epoch = @claim_epoch\b`)
	movesClaimEpoch    = regexp.MustCompile(`\bclaim_epoch = claim_epoch\b ?([^,\n]*)`)
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

func TestEveryQueryOfARunsHolderIsFencedOnItsClaimEpoch(t *testing.T) {
	queries := namedQueries(t)
	for _, name := range holderQueries {
		query, found := queries[name]
		if !found {
			t.Errorf("query %s is gone; name the query that replaced it in holderQueries", name)
			continue
		}
		if !fencedOnClaimEpoch.MatchString(query) {
			t.Errorf("%s does not compare claim_epoch to the epoch its caller was handed, so a claim that was released and taken again passes it", name)
		}
	}
}

func TestOnlyAClaimMovesTheClaimEpochAndOnlyUp(t *testing.T) {
	queries := namedQueries(t)
	for name, query := range queries {
		for _, move := range movesClaimEpoch.FindAllStringSubmatch(query, -1) {
			if name != claimQuery || strings.TrimSpace(move[1]) != "+ 1" {
				t.Errorf("%s sets claim_epoch = claim_epoch %s; only %s moves it, by + 1, or two claims could hold the same epoch", name, move[1], claimQuery)
			}
		}
	}
	if !movesClaimEpoch.MatchString(queries[claimQuery]) {
		t.Errorf("%s does not raise claim_epoch, so the claim after a release holds the epoch of the released one", claimQuery)
	}
}
