package revisions_test

import (
	"context"
	"testing"

)

func TestClaimStillWorksWhenBuildRetentionCleanupFails(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	d.Exec(t, `CREATE FUNCTION refuse_build_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cleanup refused'; END $$`)
	d.Exec(t, "CREATE TRIGGER refuse_build_delete BEFORE DELETE ON agent_request_builds FOR EACH STATEMENT EXECUTE FUNCTION refuse_build_delete()")
	row, err := svc.SubmitObserve(context.Background(), user, "claim anyway")
	if err != nil {
		t.Fatal(err)
	}

	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != row.ID {
		t.Fatalf("claim = %+v, found %v; want the queued request despite the failing cleanup", claimed, found)
	}
}
