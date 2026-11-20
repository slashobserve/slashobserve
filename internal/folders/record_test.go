package folders

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func groupGrant(name, level string) dbsqlc.ListFolderGrantsForAuditRow {
	return dbsqlc.ListFolderGrantsForAuditRow{GroupName: pgtype.Text{String: name, Valid: true}, Level: level}
}

func TestAPermissionChangeNamesWhatWasAddedRemovedAndChanged(t *testing.T) {
	before := []dbsqlc.ListFolderGrantsForAuditRow{groupGrant("sre", "view"), groupGrant("ops", "edit"), groupGrant("qa", "view")}
	after := []dbsqlc.ListFolderGrantsForAuditRow{groupGrant("sre", "edit"), groupGrant("ops", "edit"), groupGrant("dev", "view")}

	got, unchanged := permissionChange(before, after)

	want := map[string]any{
		"added":        []recordedGrant{{Kind: "group", Name: "dev", Level: "view"}},
		"removed":      []recordedGrant{{Kind: "group", Name: "qa", Level: "view"}},
		"changed":      []changedGrant{{recordedGrant: recordedGrant{Kind: "group", Name: "sre", Level: "edit"}, From: "view"}},
		"count_before": 3,
		"count_after":  3,
		"truncated":    false,
	}
	if unchanged || !reflect.DeepEqual(got, want) {
		t.Fatalf("change = %+v, want %+v", got, want)
	}
}

func TestAnIdenticalPermissionListChangesNothing(t *testing.T) {
	list := []dbsqlc.ListFolderGrantsForAuditRow{groupGrant("sre", "view")}

	got, unchanged := permissionChange(list, list)

	if !unchanged || len(got["added"].([]recordedGrant))+len(got["removed"].([]recordedGrant))+len(got["changed"].([]changedGrant)) != 0 {
		t.Fatalf("change = %+v, want no entries", got)
	}
}

func TestAPermissionChangeStoresAtMostTwoHundredEntriesAndSaysSo(t *testing.T) {
	var after []dbsqlc.ListFolderGrantsForAuditRow
	for i := range maxRecordedGrants + 25 {
		after = append(after, groupGrant(fmt.Sprintf("team-%03d", i), "view"))
	}

	got, _ := permissionChange(nil, after)

	if n := len(got["added"].([]recordedGrant)); n != maxRecordedGrants {
		t.Fatalf("stored %d entries, want %d", n, maxRecordedGrants)
	}
	if got["truncated"] != true || got["count_after"] != maxRecordedGrants+25 || got["count_before"] != 0 {
		t.Fatalf("change = counts %v to %v, truncated %v; want the full count and truncated", got["count_before"], got["count_after"], got["truncated"])
	}
}
