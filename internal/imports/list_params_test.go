package imports_test

import (
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/imports"
)

func TestListParamsOf(t *testing.T) {
	caller := auth.Principal{Kind: auth.KindUser, ID: "6f1c1b3e-5f4a-4a53-9d63-0f2f6a3f8a11", Name: "Ada"}
	dashboardID := "0d0a4d2e-7b9c-4d55-8a1e-2c9f3b6a7e42"
	parsed, err := db.ParseUUID(dashboardID)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		query     string
		madeBy    bool
		dashboard pgtype.UUID
		invalid   bool
	}{
		{name: "no parameters"},
		{name: "made by me", query: "made_by=me", madeBy: true},
		{name: "empty made_by", query: "made_by=", invalid: true},
		{name: "someone else", query: "made_by=someone", invalid: true},
		{name: "repeated made_by", query: "made_by=me&made_by=you", invalid: true},
		{name: "dashboard id", query: "dashboard_id=" + dashboardID, dashboard: parsed},
		{name: "empty dashboard_id", query: "dashboard_id=", invalid: true},
		{name: "dashboard_id that is not a UUID", query: "dashboard_id=nope", invalid: true},
		{name: "both filters", query: "made_by=me&dashboard_id=" + dashboardID, madeBy: true, dashboard: parsed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := imports.ListParamsOf(query, caller)
			if tt.invalid {
				if !errors.Is(err, imports.ErrInvalidFilter) {
					t.Fatalf("err = %v, want ErrInvalidFilter", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.madeBy != (got.MadeBy != nil) || (tt.madeBy && !reflect.DeepEqual(*got.MadeBy, caller)) {
				t.Fatalf("MadeBy = %v, want set %v", got.MadeBy, tt.madeBy)
			}
			if got.DashboardID != tt.dashboard {
				t.Fatalf("DashboardID = %v, want %v", got.DashboardID, tt.dashboard)
			}
		})
	}
}
