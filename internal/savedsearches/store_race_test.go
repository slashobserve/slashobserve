package savedsearches

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestUpdateOfASearchThatLeftTheCheckedFolderIsRefused(t *testing.T) {
	d := dbtest.New(t)
	var searchID, checked, elsewhere string
	sql := d.SQLDB(t)
	if err := sql.QueryRowContext(reading(t.Context()), `SELECT id::text FROM folders WHERE general`).Scan(&checked); err != nil {
		t.Fatal(err)
	}
	if err := sql.QueryRowContext(reading(t.Context()), `INSERT INTO folders (title) VALUES ('Elsewhere') RETURNING id::text`).Scan(&elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := sql.QueryRowContext(reading(t.Context()), `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name)
		VALUES ('logs', 'Errors', 'loki', '{a="b"}', 'now-1h', 'now', $1::uuid, 'user', gen_random_uuid(), 'Eve') RETURNING id::text`, elsewhere).Scan(&searchID); err != nil {
		t.Fatal(err)
	}
	id, _ := db.ParseUUID(searchID)
	checkedFolder, _ := db.ParseUUID(checked)
	title := "renamed"

	_, err := (&store{db: d.DB}).update(reading(t.Context()), actor.Actor{}, id, checkedFolder, change{title: &title})

	if !errors.Is(err, errFolderMoved) {
		t.Fatalf("update = %v, want errFolderMoved because the search is no longer in the folder whose access was checked", err)
	}
	var got string
	if err := sql.QueryRowContext(reading(t.Context()), `SELECT title FROM saved_searches WHERE id = $1::uuid`, searchID).Scan(&got); err != nil || got != "Errors" {
		t.Fatalf("title = %q (%v), want it untouched", got, err)
	}
}
