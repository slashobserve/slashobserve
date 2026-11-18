package health_test

import "testing"

func TestViewsCountOnePersonOncePerDay(t *testing.T) {
	r := newRig(t)
	id := r.create(t, boardSpec("Checkout", "up"))

	for range 3 {
		if err := r.svc.RecordView(t.Context(), r.viewer, id); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.svc.Views(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || got.WindowDays != 30 {
		t.Fatalf("views after one person opened it three times = %+v, want 1 over 30 days", got)
	}

	if err := r.svc.RecordView(t.Context(), r.editor, id); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.svc.Views(t.Context(), id); got.Count != 2 {
		t.Fatalf("views after a second person opened it = %d, want 2", got.Count)
	}
}

func TestViewsCountEachDayOfAPerson(t *testing.T) {
	r := newRig(t)
	id := r.create(t, boardSpec("Checkout", "up"))
	if err := r.svc.RecordView(t.Context(), r.viewer, id); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `INSERT INTO dashboard_view_days (dashboard_id, user_id, day) VALUES ($1, $2, current_date - 1), ($1, $2, current_date - 29)`, id.String(), r.viewer.ID)
	r.db.Exec(t, `INSERT INTO dashboard_view_days (dashboard_id, user_id, day) VALUES ($1, $2, current_date - 30), ($1, $2, current_date - 200)`, id.String(), r.viewer.ID)

	got, err := r.svc.Views(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 3 {
		t.Fatalf("views = %d, want today, yesterday and 29 days ago, and not the days outside the window", got.Count)
	}
}

func TestViewsIgnoreAKeyAndAnUnknownDashboard(t *testing.T) {
	r := newRig(t)
	id := r.create(t, boardSpec("Checkout", "up"))
	if err := r.svc.RecordView(t.Context(), r.key, id); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.svc.Views(t.Context(), id); got.Count != 0 {
		t.Fatalf("a key's open counted: %d", got.Count)
	}
	gone := r.create(t, boardSpec("Gone", "up"))
	r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1`, gone.String())
	if err := r.svc.RecordView(t.Context(), r.viewer, gone); err != nil {
		t.Fatalf("recording a view of a deleted dashboard failed: %v", err)
	}
}

func TestRunPrunesViewDaysOlderThanNinetyDays(t *testing.T) {
	r := newRig(t)
	id := r.create(t, boardSpec("Checkout", "up"))
	r.db.Exec(t, `INSERT INTO dashboard_view_days (dashboard_id, user_id, day) VALUES ($1, $2, current_date - 89), ($1, $2, current_date - 91), ($1, $2, current_date - 400)`, id.String(), r.viewer.ID)

	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := r.db.SQLDB(t).QueryRow(`SELECT count(*) FROM dashboard_view_days`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("%d view days left, want only the one 89 days old", left)
	}
}
