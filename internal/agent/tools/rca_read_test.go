package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestListRCAsAndGetRCAReadThePublishedRevisions(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "Errors began at 09:12"))
	r.published(t, editorCaller, id)
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "Only checkout fails"))
	r.published(t, editorCaller, id)

	list, ok := r.call(t, viewerCaller, nameListRCAs, fmt.Sprintf(`{"observation_id":%q}`, id)).Content.(rcaListOut)
	if !ok || len(list.RCAs) != 2 || list.RCAs[0].Revision != 2 || list.RCAs[1].Revision != 1 || list.Truncated {
		t.Fatalf("list = %#v", list)
	}
	if list.RCAs[0].Title != "Checkout 5xx" || list.RCAs[0].ObservationState != "open" || list.RCAs[0].CreatedBy.Name != editorCaller.Name {
		t.Errorf("newest revision = %#v", list.RCAs[0])
	}

	first := r.call(t, viewerCaller, nameGetRCA, fmt.Sprintf(`{"observation_id":%q,"revision":1}`, id))
	got, ok := first.Content.(rcaReadOut)
	if first.IsError || !ok || got.Revision != 1 || got.ObservationID != id || len(got.Blocks) != 1 || got.Blocks[0].Markdown != "Errors began at 09:12" || got.Truncated {
		t.Fatalf("revision 1 = %s", textOf(t, first))
	}
	if second := r.call(t, viewerCaller, nameGetRCA, fmt.Sprintf(`{"observation_id":%q,"revision":2}`, id)).Content.(rcaReadOut); len(second.Blocks) != 2 {
		t.Errorf("revision 2 has %d blocks, want 2", len(second.Blocks))
	}
}

func TestGetRCAKeepsTheBlocksThatFitWhenTheRevisionIsTooLarge(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	chunk := strings.Repeat("x", 3000)
	for range 8 {
		r.written(t, editorCaller, nameAddBlock, textBlockInput(id, chunk))
	}
	r.published(t, editorCaller, id)

	res := r.call(t, viewerCaller, nameGetRCA, fmt.Sprintf(`{"observation_id":%q,"revision":1}`, id))
	got, ok := res.Content.(rcaReadOut)
	if res.IsError || !ok || !got.Truncated || len(got.Blocks) == 0 || len(got.Blocks) >= 8 || jsonSize(got) > listBudget {
		t.Fatalf("large revision = %d blocks, truncated %v, %d bytes", len(got.Blocks), got.Truncated, jsonSize(got))
	}
}

func TestReadingRCAsRefusesWhatDoesNotExist(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "text"))
	r.published(t, editorCaller, id)

	missing := r.call(t, viewerCaller, nameGetRCA, fmt.Sprintf(`{"observation_id":%q,"revision":9}`, id))
	if body, _ := missing.Content.(errorBody); errorCode(t, missing) != codeNotFound || body.Message != "RCA revision not found" {
		t.Errorf("revision 9 = %#v", missing.Content)
	}
	unknown := "99999999-9999-4999-8999-999999999999"
	for tool, input := range map[string]string{
		nameListRCAs: fmt.Sprintf(`{"observation_id":%q}`, unknown),
		nameGetRCA:   fmt.Sprintf(`{"observation_id":%q,"revision":1}`, unknown),
	} {
		if code := errorCode(t, r.call(t, viewerCaller, tool, input)); code != codeNotFound {
			t.Errorf("%s of an unknown observation answered %s, want not_found", tool, code)
		}
	}
	if code := errorCode(t, r.call(t, viewerCaller, nameGetRCA, fmt.Sprintf(`{"observation_id":%q}`, id))); code != codeInvalidRequest {
		t.Errorf("get_rca without a revision answered %s, want invalid_request", code)
	}
}

func TestReadingRCAsNeedsObservations(t *testing.T) {
	for _, tool := range []string{nameListRCAs, nameGetRCA} {
		toolFailsInternally(t, New(Deps{}), tool, `{"observation_id":"99999999-9999-4999-8999-999999999999","revision":1}`)
	}
}

func toolFailsInternally(t *testing.T, r *Registry, tool, input string) {
	t.Helper()
	res, err := r.Call(auth.WithPrincipal(t.Context(), editorCaller), tool, []byte(input))
	if err != nil || errorCode(t, res) != FailureToolFailed {
		t.Errorf("%s without its service: res = %#v, err = %v", tool, res.Content, err)
	}
}
