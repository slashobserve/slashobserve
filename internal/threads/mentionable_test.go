package threads

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r *rig) createUserID(t *testing.T, name, email string) string {
	t.Helper()
	return r.createUser(t, name, email).String()
}

func names(people []Person) string {
	out := make([]string, len(people))
	for i, p := range people {
		out[i] = p.Name
	}
	return strings.Join(out, ",")
}

func TestMentionablePeopleAreThoseWhoCanSeeTheTarget(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	r.createUser(t, "Zed Zimmer", "zed@example.com")
	r.createUser(t, "Amy Ames", "amy@example.com")
	r.createUser(t, "", "nameless@example.com")
	tg := target(KindDashboard, dash)

	all, _, err := r.service.MentionablePeople(t.Context(), testUser, tg, "", 20)
	if err != nil || names(all) != "Amy Ames,nameless@example.com,Zed Zimmer" {
		t.Fatalf("everyone = %s, err %v", names(all), err)
	}
	if byName, _, err := r.service.MentionablePeople(t.Context(), testUser, tg, "ZIM", 20); err != nil || names(byName) != "Zed Zimmer" {
		t.Fatalf("by name = %s, err %v", names(byName), err)
	}
	if byEmail, _, err := r.service.MentionablePeople(t.Context(), testUser, tg, "amy@", 20); err != nil || names(byEmail) != "Amy Ames" {
		t.Fatalf("by email = %s, err %v", names(byEmail), err)
	}
	if wildcard, _, err := r.service.MentionablePeople(t.Context(), testUser, tg, "%", 20); err != nil || len(wildcard) != 0 {
		t.Fatalf("a literal percent sign must match nothing: %s, err %v", names(wildcard), err)
	}
	if capped, _, err := r.service.MentionablePeople(t.Context(), testUser, tg, "", 2); err != nil || len(capped) != 2 {
		t.Fatalf("limit 2 = %s, err %v", names(capped), err)
	}
}

func TestMentionablePeopleOfARestrictedTargetAreItsGroupAndAdmins(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	foldertest.Move(t, r.db, dash, restricted)
	r.createUser(t, "Outsider", "outsider@example.com")
	insider := r.createUser(t, "Insider", "insider@example.com")
	r.db.Exec(t, `UPDATE users SET groups = ARRAY[$2] WHERE id = $1`, insider, payrollGroup)
	boss := r.createUser(t, "Boss", "boss@example.com")
	r.db.Exec(t, `UPDATE users SET role = 'admin' WHERE id = $1`, boss)
	tg := target(KindDashboard, dash)

	people, _, err := r.service.MentionablePeople(t.Context(), admin, tg, "", 20)
	if err != nil || names(people) != "Boss,Insider" {
		t.Fatalf("people = %s, err %v", names(people), err)
	}

	_, message := r.open(t, admin, OpenInput{Target: tg, Body: "look @[x](user:" + insider.String() + ")"})
	if len(message.Mentions) != 1 || message.Mentions[0].UserID != insider {
		t.Fatalf("recorded mentions = %+v, want only the insider", message.Mentions)
	}
	_, _, err = r.service.Open(t.Context(), admin, OpenInput{Target: tg, Body: "look @[y](user:" + r.createUserID(t, "Late", "late@example.com") + ")"}, nil)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidMention {
		t.Fatalf("a person who cannot see the target: err = %v, want invalid_mention", err)
	}
}

func TestMentionablePeopleNeedTheCommentAbilityAndAVisibleTarget(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	tg := target(KindDashboard, dash)
	if _, _, err := r.service.MentionablePeople(t.Context(), viewer, tg, "", 20); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a viewer: err = %v, want folders.ErrForbidden", err)
	}
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	foldertest.Move(t, r.db, dash, restricted)
	if _, _, err := r.service.MentionablePeople(t.Context(), commenter, tg, "", 20); !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("a hidden target: err = %v, want ErrTargetNotFound", err)
	}
}

func TestMentionablePeopleThroughTheRoute(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	r.createUser(t, "Zed Zimmer", "zed@example.com")
	path := "/threads/mentionable?target=dashboard:" + dash.String()

	ok := r.call(t, commenter, http.MethodGet, path+"&q=zed", nil)
	ok.expect(t, http.StatusOK, "")
	people, _ := ok.at(t, "people").([]any)
	if len(people) != 1 || people[0].(map[string]any)["name"] != "Zed Zimmer" || people[0].(map[string]any)["email"] != "zed@example.com" {
		t.Fatalf("people = %v", people)
	}
	r.call(t, viewer, http.MethodGet, path, nil).expect(t, http.StatusForbidden, "")
	r.call(t, commenter, http.MethodGet, "/threads/mentionable?target=nope", nil).expect(t, http.StatusUnprocessableEntity, "invalid_target")
	r.call(t, commenter, http.MethodGet, path+"&q="+url.QueryEscape(strings.Repeat("a", 101)), nil).expect(t, http.StatusBadRequest, "")
}

func TestMentionablePeopleOfARestrictedTargetAreFoundBeyondTheFirstScannedPage(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	foldertest.Move(t, r.db, dash, restricted)
	r.db.Exec(t, `INSERT INTO users (oidc_issuer, oidc_subject, email, name)
		SELECT 'https://idp', 'bulk-' || n, 'bulk-' || n || '@example.com', 'Aaa ' || lpad(n::text, 4, '0') FROM generate_series(1, $1::int) n`, mentionableScanRows+20)
	late := r.createUser(t, "Zed Late", "zed@example.com")
	r.db.Exec(t, `UPDATE users SET groups = ARRAY[$2] WHERE id = $1`, late, payrollGroup)

	people, more, err := r.service.MentionablePeople(t.Context(), admin, target(KindDashboard, dash), "", 20)
	if err != nil || names(people) != "Zed Late" || more {
		t.Fatalf("people = %s, more = %v, err %v, want the group member past the first page and no more", names(people), more, err)
	}
}

func TestEditKeepsAMentionOfSomeoneWhoCanNoLongerSeeTheTarget(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	foldertest.Move(t, r.db, dash, restricted)
	insider := r.createUser(t, "Insider", "insider@example.com")
	r.db.Exec(t, `UPDATE users SET groups = ARRAY[$2] WHERE id = $1`, insider, payrollGroup)
	outsider := r.createUserID(t, "Outsider", "outsider@example.com")
	tg := target(KindDashboard, dash)
	token := "@[x](user:" + insider.String() + ")"
	_, first := r.open(t, admin, OpenInput{Target: tg, Body: "look " + token})

	r.db.Exec(t, `UPDATE users SET groups = '{}' WHERE id = $1`, insider)

	_, edited, changed, err := r.service.Edit(t.Context(), admin, mustParse(t, first.ID), "look again "+token)
	if err != nil || !changed || len(edited.Mentions) != 1 || edited.Mentions[0].UserID != insider {
		t.Fatalf("edit keeping the mention: mentions = %+v, changed = %v, err = %v", edited.Mentions, changed, err)
	}
	_, _, _, err = r.service.Edit(t.Context(), admin, mustParse(t, first.ID), "look "+token+" @[y](user:"+outsider+")")
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidMention {
		t.Fatalf("edit adding a person who cannot see it: err = %v, want invalid_mention", err)
	}
}
