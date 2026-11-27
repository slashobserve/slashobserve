package provision

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
)

func TestOnlyAVersionTheProvisionerDidNotSaveCountsAsReplaced(t *testing.T) {
	own := dashboards.VersionMeta{Number: 3, Author: dashboards.Author{Kind: string(Principal.Kind), ID: Principal.ID, Name: Principal.Name}}
	if got := versionReplaced(own); got != 0 {
		t.Errorf("a file change over the provisioner's own version 3 replaces version %d, want none", got)
	}
	theirs := dashboards.VersionMeta{Number: 3, Author: dashboards.Author{Kind: "user", ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "Ada"}}
	if got := versionReplaced(theirs); got != 3 {
		t.Errorf("a file change over Ada's version 3 replaces version %d, want 3", got)
	}
}
