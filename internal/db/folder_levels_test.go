package db_test

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestFolderPermissionLevels(t *testing.T) {
	sqlDB := dbtest.New(t).SQLDB(t)
	var folderID string
	if err := sqlDB.QueryRow(`INSERT INTO folders (title) VALUES ('Levels') RETURNING id`).Scan(&folderID); err != nil {
		t.Fatalf("insert folder: %v", err)
	}
	for _, level := range []string{"view", "comment", "edit", "admin"} {
		t.Run(level+" is accepted", func(t *testing.T) {
			if _, err := sqlDB.Exec(`INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, $2, $3)`, folderID, "group-"+level, level); err != nil {
				t.Errorf("insert %s row: %v", level, err)
			}
		})
	}
	t.Run("owner is refused", func(t *testing.T) {
		_, err := sqlDB.Exec(`INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, 'group-owner', 'owner')`, folderID)
		if err == nil || !strings.Contains(err.Error(), "folder_permissions_level_check") {
			t.Errorf("error = %v, want a folder_permissions_level_check violation", err)
		}
	})
}
