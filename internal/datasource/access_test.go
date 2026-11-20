package datasource_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

var adminsOnlyRegistrations = map[string]func(t *testing.T) *datasource.Registry{
	"built in": withAdminsOnly,
	"from the file": func(t *testing.T) *datasource.Registry {
		registry, err := datasource.NewRegistry(nil,
			datasource.Instance{UID: "prom", Datasource: failingHealth{}},
			datasource.Instance{UID: "audit", Audience: datasource.AudienceAdmins, Datasource: failingHealth{}},
		)
		if err != nil {
			t.Fatal(err)
		}
		return registry
	},
	"added by an Admin": func(t *testing.T) *datasource.Registry {
		registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: failingHealth{}})
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Put(datasource.Instance{UID: "audit", Audience: datasource.AudienceAdmins, Datasource: failingHealth{}}); err != nil {
			t.Fatal(err)
		}
		return registry
	},
}

func withAdminsOnly(t *testing.T) *datasource.Registry {
	t.Helper()
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: failingHealth{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: failingHealth{}}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func user(role auth.Role) auth.Principal {
	return auth.Principal{Kind: auth.KindUser, ID: "u-" + string(role), Role: role}
}

func key(role auth.Role) auth.Principal {
	return auth.Principal{Kind: auth.KindServiceAccount, ID: "k-" + string(role), Role: role}
}

func TestAnAdminsOnlyDatasourceIsReadOnlyByAnAdminOrTheSystem(t *testing.T) {
	for how, register := range adminsOnlyRegistrations {
		t.Run(how, func(t *testing.T) { checkAdminsOnlyMatrix(t, register(t)) })
	}
}

func checkAdminsOnlyMatrix(t *testing.T, registry *datasource.Registry) {
	cases := []struct {
		name   string
		access datasource.Access
		reads  bool
	}{
		{"viewer", datasource.Access{By: user(auth.RoleViewer)}, false},
		{"commenter", datasource.Access{By: user(auth.RoleCommenter)}, false},
		{"editor", datasource.Access{By: user(auth.RoleEditor)}, false},
		{"admin", datasource.Access{By: user(auth.RoleAdmin)}, true},
		{"editor service account key", datasource.Access{By: key(auth.RoleEditor)}, false},
		{"admin service account key", datasource.Access{By: key(auth.RoleAdmin)}, true},
		{"anonymous public link", datasource.Access{}, false},
		{"system", datasource.Access{System: true}, true},
		{"admin writing for everyone", datasource.Access{By: user(auth.RoleAdmin), Audience: datasource.AudienceEveryone}, false},
		{"admin writing for admins", datasource.Access{By: user(auth.RoleAdmin), Audience: datasource.AudienceAdmins}, true},
		{"viewer writing for admins", datasource.Access{By: user(auth.RoleViewer), Audience: datasource.AudienceAdmins}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := datasource.WithAccess(t.Context(), tc.access)
			if _, err := registry.For(ctx, "prom"); err != nil {
				t.Fatalf("everyone datasource: %v", err)
			}
			_, err := registry.For(ctx, "audit")
			if tc.reads != (err == nil) {
				t.Fatalf("For(audit) err = %v, want readable %v", err, tc.reads)
			}
			if !tc.reads && !errors.Is(err, datasource.ErrNotVisible) {
				t.Fatalf("err = %v, want ErrNotVisible", err)
			}
			_, missing := registry.For(ctx, "missing")
			if !tc.reads && err.Error() != missing.Error() {
				t.Fatalf("restricted error %q differs from the missing one %q", err, missing)
			}
			if got := uids(registry.Visible(ctx)); slices.Contains(got, "audit") != tc.reads || !slices.Contains(got, "prom") {
				t.Fatalf("Visible = %v", got)
			}
			var health []string
			for _, s := range registry.Health(ctx) {
				health = append(health, s.UID)
			}
			if slices.Contains(health, "audit") != tc.reads {
				t.Fatalf("Health = %v", health)
			}
			if _, ok := registry.Recheck(ctx, "audit"); ok != tc.reads {
				t.Fatalf("Recheck(audit) = %v", ok)
			}
			if datasource.ReadsRestricted(ctx) != tc.reads {
				t.Fatalf("ReadsRestricted = %v", !tc.reads)
			}
		})
	}
}

func TestALookupWithoutAccessFails(t *testing.T) {
	registry := withAdminsOnly(t)
	if _, err := registry.For(t.Context(), "prom"); !errors.Is(err, datasource.ErrNoAccess) || errors.Is(err, datasource.ErrNotVisible) {
		t.Fatalf("For without access err = %v, want ErrNoAccess", err)
	}
	if got := registry.Visible(t.Context()); got != nil {
		t.Fatalf("Visible without access = %v", uids(got))
	}
	if got := registry.Health(t.Context()); len(got) != 0 {
		t.Fatalf("Health without access = %v", got)
	}
	if datasource.ReadsRestricted(t.Context()) {
		t.Fatal("ReadsRestricted without access")
	}
}

func TestAnAdminsOnlyDatasourceCannotBeReplacedRemovedOrDuplicated(t *testing.T) {
	registry := withAdminsOnly(t)
	if !registry.AdminsOnly("audit") || registry.AdminsOnly("prom") || registry.AdminsOnly("missing") {
		t.Fatal("AdminsOnly answers the wrong datasources")
	}
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: failingHealth{}}); err == nil {
		t.Fatal("a second AddAdminsOnly with the same uid succeeded")
	}
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "prom", Datasource: failingHealth{}}); err == nil {
		t.Fatal("AddAdminsOnly replaced an everyone datasource")
	}
	if err := registry.Put(datasource.Instance{UID: "audit", Datasource: failingHealth{}}); err == nil {
		t.Fatal("Put replaced the admins only datasource")
	}
	registry.Remove("audit")
	if !registry.AdminsOnly("audit") {
		t.Fatal("Remove deleted the admins only datasource")
	}
	in, err := registry.For(datasource.WithAccess(t.Context(), datasource.Access{System: true}), "audit")
	if err != nil || in.Source != datasource.SourceFile {
		t.Fatalf("audit = %+v, %v; want a read-only file source", in, err)
	}
}

func TestADatasourceWithNoAudienceIsForEveryone(t *testing.T) {
	registry := withAdminsOnly(t)
	if err := registry.Put(datasource.Instance{UID: "added", Datasource: failingHealth{}}); err != nil {
		t.Fatal(err)
	}
	if registry.AdminsOnly("added") || registry.AdminsOnly("prom") {
		t.Fatal("a datasource with no audience is admins only")
	}
	if _, err := registry.For(datasource.WithAccess(t.Context(), datasource.Access{By: user(auth.RoleViewer)}), "added"); err != nil {
		t.Fatalf("viewer cannot read an Admin-added datasource: %v", err)
	}
}

func uids(list []datasource.Instance) []string {
	out := make([]string, 0, len(list))
	for _, in := range list {
		out = append(out, in.UID)
	}
	return out
}

func TestAnAudienceThatIsNotOneIsRefused(t *testing.T) {
	if _, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Audience: "Admins", Datasource: failingHealth{}}); err == nil {
		t.Fatal("NewRegistry accepted an unknown audience")
	}
	registry := withAdminsOnly(t)
	if err := registry.Put(datasource.Instance{UID: "added", Audience: "nobody", Datasource: failingHealth{}}); err == nil {
		t.Fatal("Put accepted an unknown audience")
	}
}

func TestAnAdminCanOpenAnAddedDatasourceToEveryoneAgain(t *testing.T) {
	registry := adminsOnlyRegistrations["added by an Admin"](t)
	if err := registry.Put(datasource.Instance{UID: "audit", Audience: datasource.AudienceEveryone, Datasource: failingHealth{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.For(datasource.WithAccess(t.Context(), datasource.Access{By: user(auth.RoleViewer)}), "audit"); err != nil {
		t.Fatalf("viewer cannot read a reopened datasource: %v", err)
	}
}
