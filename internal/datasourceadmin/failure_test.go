package datasourceadmin

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestEveryCauseOfALoadFailureHasItsOwnCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{datasource.ErrServerOwned, codeServerOwned},
		{fmt.Errorf("put: %w", datasource.ErrServerOwned), codeServerOwned},
		{datasource.Errorf(datasource.ErrNoSecretKey, "no secret key is set"), codeNoSealingKey},
		{datasource.Errorf(datasource.ErrUnreadable, "the sealed credentials cannot be read"), codeUnreadable},
		{datasource.Errorf(datasource.ErrNotLoadable, definitionNotJSON), codeDefinitionJSON},
		{datasource.Errorf(datasource.ErrNotLoadable, definitionInvalid), codeDefinitionBad},
		{datasource.Errorf(datasource.ErrNotLoadable, definitionUnopened), codeDefinitionOpens},
		{datasource.Errorf(datasource.ErrNotLoadable, "something else"), codeNotLoadable},
		{datasource.ErrNotLoadable, codeNotLoadable},
		{errors.New(`datasource uid "x y" must match a pattern`), codeNotLoaded},
		{datasource.Errorf(datasource.ErrUnavailable, "down"), codeNotLoaded},
	}
	for _, c := range cases {
		if got := failureCode(c.err); got != c.want {
			t.Errorf("failureCode(%v) = %s, want %s", c.err, got, c.want)
		}
	}
}

func TestAStoredDatasourceThatFailedToLoadIsTriedAgainOnlyWhenItsRowChanges(t *testing.T) {
	file := datasource.Instance{UID: "prom", Name: "From file", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	r, svc := newRig(t, testKey, file)
	var logs bytes.Buffer
	svc.logger = slog.New(slog.NewTextHandler(&logs, nil))
	row := r.insert(t, testKey, prometheus("prom", "http://prometheus.example:9090"), Secrets{})

	for range 3 {
		if err := svc.Reconcile(reading(t.Context())); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(logs.String(), "stored datasource not loaded"); n != 1 {
		t.Fatalf("%d failure lines over three polls, want one:\n%s", n, logs.String())
	}
	if !strings.Contains(logs.String(), "error_code="+codeServerOwned) || strings.Contains(logs.String(), "reason=") {
		t.Fatalf("the failure is not logged by its code alone:\n%s", logs.String())
	}
	if v := svc.applied["prom"]; !v.failed {
		t.Fatalf("applied version %+v, want it marked failed", v)
	}

	r.database.Exec(t, "UPDATE datasources SET updated_at = clock_timestamp() WHERE id = $1", row.ID)
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(logs.String(), "stored datasource not loaded"); n != 2 {
		t.Fatalf("%d failure lines after the row changed, want a second try:\n%s", n, logs.String())
	}
}
