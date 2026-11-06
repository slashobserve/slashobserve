package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"regexp"
	"slices"
	"strings"
)

const (
	inputHashChars      = 12
	maxLoggedIssuePaths = 20
)

var issuePathPattern = regexp.MustCompile(`^(/[A-Za-z0-9_-]{1,64})*$`)

type loggedPanel struct {
	ID     string        `json:"id"`
	Panels []loggedPanel `json:"panels"`
}

type loggedSave struct {
	Spec struct {
		Panels []loggedPanel `json:"panels"`
	} `json:"spec"`
	Upsert []loggedPanel `json:"upsert"`
	Delete []string      `json:"delete"`
	Layout []loggedPanel `json:"layout"`
}

type loggedRejection struct {
	Issues []struct {
		Path string `json:"path"`
	} `json:"issues"`
}

func logToolInput(ctx context.Context, logger *slog.Logger, tool string, input json.RawMessage, rejected bool, content []byte) {
	sum := sha256.Sum256(input)
	attrs := []any{
		agentRequestAttr(ctx),
		slog.String("tool", tool),
		slog.String("input_sha256", hex.EncodeToString(sum[:])[:inputHashChars]),
		slog.Int("input_bytes", len(input)),
	}
	if savesPanels(tool) {
		ids := savedPanelIDs(input)
		attrs = append(attrs, slog.String("panel_ids", strings.Join(ids, ",")), slog.Int("panel_count", len(ids)))
		if rejected {
			attrs = append(attrs, slog.Any("issue_paths", issuePaths(content)))
		}
	}
	logger.InfoContext(ctx, "agent tool input", attrs...)
}

func savesPanels(tool string) bool {
	return isSave(tool) && tool != toolImportGrafana
}

func savedPanelIDs(input json.RawMessage) []string {
	var save loggedSave
	if json.Unmarshal(input, &save) != nil {
		return []string{}
	}
	ids := []string{}
	add := func(id string) {
		if panelIDPattern.MatchString(id) && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	var walk func([]loggedPanel)
	walk = func(panels []loggedPanel) {
		for _, p := range panels {
			add(p.ID)
			walk(p.Panels)
		}
	}
	walk(save.Spec.Panels)
	walk(save.Upsert)
	walk(save.Layout)
	for _, id := range save.Delete {
		add(id)
	}
	return ids
}

func issuePaths(content []byte) []string {
	var rejection loggedRejection
	paths := []string{}
	if json.Unmarshal(content, &rejection) != nil {
		return paths
	}
	for _, issue := range rejection.Issues {
		if len(paths) == maxLoggedIssuePaths {
			break
		}
		if issuePathPattern.MatchString(issue.Path) {
			paths = append(paths, issue.Path)
		}
	}
	return paths
}
