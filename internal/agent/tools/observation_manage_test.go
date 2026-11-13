package tools

import (
	"fmt"
	"strings"
	"testing"
)

func (r *observationRig) managed(t *testing.T, tool, input string) observationManagedOut {
	t.Helper()
	res := r.call(t, editorCaller, tool, input)
	out, ok := res.Content.(observationManagedOut)
	if res.IsError || !ok {
		t.Fatalf("%s %s: %#v", tool, input, res.Content)
	}
	return out
}

func rangeOf(page observationViewOut) string {
	if page.From == nil || page.To == nil {
		return "none"
	}
	return *page.From + " to " + *page.To
}

func TestUpdateObservationChangesTheTitleAndTheRange(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)

	renamed := r.managed(t, nameUpdateObservation, fmt.Sprintf(`{"observation_id":%q,"title":"Checkout 5xx, root cause found"}`, id))
	if renamed.Title != "Checkout 5xx, root cause found" || renamed.State != "open" || renamed.FolderID != r.folder || !strings.HasSuffix(renamed.URL, "/o/"+id) {
		t.Fatalf("renamed = %#v", renamed)
	}
	if page := r.view(t, viewerCaller, id); page.Title != "Checkout 5xx, root cause found" {
		t.Errorf("get_observation title = %q", page.Title)
	}

	r.managed(t, nameUpdateObservation, fmt.Sprintf(`{"observation_id":%q,"from":"2026-11-01T09:00:00Z","to":"2026-11-01T10:00:00Z"}`, id))
	if page := r.view(t, viewerCaller, id); rangeOf(page) != "2026-11-01T09:00:00Z to 2026-11-01T10:00:00Z" {
		t.Errorf("range after the update = %q", rangeOf(page))
	}
	r.managed(t, nameUpdateObservation, fmt.Sprintf(`{"observation_id":%q,"clear_range":true}`, id))
	if page := r.view(t, viewerCaller, id); rangeOf(page) != "none" {
		t.Errorf("range after clearing = %q", rangeOf(page))
	}
}

func TestUpdateObservationRefusesBadInput(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	tests := []struct {
		name, input, message string
	}{
		{"nothing to change", fmt.Sprintf(`{"observation_id":%q}`, id), "no change given: send title, from and to, or clear_range"},
		{"clear_range false alone", fmt.Sprintf(`{"observation_id":%q,"clear_range":false}`, id), "no change given: send title, from and to, or clear_range"},
		{"a range and clear_range", fmt.Sprintf(`{"observation_id":%q,"from":"2026-11-01T09:00:00Z","to":"2026-11-01T10:00:00Z","clear_range":true}`, id), "send from and to, or clear_range, not both"},
		{"from without to", fmt.Sprintf(`{"observation_id":%q,"from":"2026-11-01T09:00:00Z"}`, id), "from and to are given together"},
		{"from after to", fmt.Sprintf(`{"observation_id":%q,"from":"2026-11-01T10:00:00Z","to":"2026-11-01T09:00:00Z"}`, id), "from must be before to"},
		{"a time that is not RFC 3339", fmt.Sprintf(`{"observation_id":%q,"from":"yesterday","to":"today"}`, id), "from must be an RFC 3339 time"},
		{"an observation id that is not a uuid", `{"observation_id":"x","title":"t"}`, "observation_id must be a UUID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := r.call(t, editorCaller, nameUpdateObservation, tt.input)
			body, _ := res.Content.(errorBody)
			if res.IsError && !strings.Contains(body.Message, tt.message) {
				t.Errorf("message = %q, want it to contain %q", body.Message, tt.message)
			}
			if !res.IsError {
				t.Errorf("accepted: %#v", res.Content)
			}
		})
	}
	if page := r.view(t, viewerCaller, id); page.Title != "Checkout 5xx" {
		t.Errorf("title after the refusals = %q", page.Title)
	}
}

func TestReopenObservationInvertsResolve(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	input := fmt.Sprintf(`{"observation_id":%q}`, id)

	notResolved := r.call(t, editorCaller, nameReopenObservation, input)
	body, _ := notResolved.Content.(errorBody)
	if errorCode(t, notResolved) != codeInvalidTransition || !strings.Contains(body.Message, "reopen is not allowed while the observation is open") {
		t.Fatalf("reopening an open observation: %#v", notResolved.Content)
	}

	r.written(t, editorCaller, nameResolveObservation, fmt.Sprintf(`{"observation_id":%q,"summary":"A bad deploy"}`, id))
	if state := r.managed(t, nameReopenObservation, input).State; state != "open" {
		t.Fatalf("state after reopen = %q, want open", state)
	}
	if page := r.view(t, viewerCaller, id); page.State != "open" {
		t.Errorf("get_observation state = %q", page.State)
	}
	if code := errorCode(t, r.call(t, viewerCaller, nameReopenObservation, input)); code != codeForbiddenRole && code != codeInvalidTransition {
		t.Errorf("a viewer reopening answered %s", code)
	}
}

func TestMoveObservationChangesItsFolder(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	var other string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ('Archive') RETURNING id::text`).Scan(&other); err != nil {
		t.Fatal(err)
	}

	moved := r.managed(t, nameMoveObservation, fmt.Sprintf(`{"observation_id":%q,"folder_id":%q}`, id, other))
	if moved.FolderID != other || moved.ObservationID != id {
		t.Fatalf("moved = %#v, want folder %s", moved, other)
	}
	if page := r.view(t, viewerCaller, id); page.FolderID != other {
		t.Errorf("get_observation folder = %q, want %q", page.FolderID, other)
	}

	unknown := r.call(t, editorCaller, nameMoveObservation, fmt.Sprintf(`{"observation_id":%q,"folder_id":"99999999-9999-4999-8999-999999999999"}`, id))
	if body, _ := unknown.Content.(errorBody); errorCode(t, unknown) != codeNotFound || body.Message != "folder not found" {
		t.Errorf("unknown folder: %#v", unknown.Content)
	}
	if code := errorCode(t, r.call(t, editorCaller, nameMoveObservation, fmt.Sprintf(`{"observation_id":%q,"folder_id":""}`, id))); code != codeInvalidRequest {
		t.Errorf("an empty folder answered %s, want invalid_request", code)
	}
}

func TestObservationManageToolsNeedObservations(t *testing.T) {
	for _, tool := range []string{nameUpdateObservation, nameMoveObservation, nameReopenObservation} {
		toolFailsInternally(t, New(Deps{}), tool, `{"observation_id":"99999999-9999-4999-8999-999999999999"}`)
	}
}
