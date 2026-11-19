package auth

import (
	"strings"
	"testing"
)

func TestInviteEmailProblemAcceptsOneAddress(t *testing.T) {
	for _, ok := range []string{"ada@example.com", "a.b+c@sub.example.org"} {
		if got := InviteEmailProblem(ok); got != "" {
			t.Errorf("%q: %q, want no problem", ok, got)
		}
	}
	for _, bad := range []string{"", "ada", "@example.com", "ada@", "ada @example.com", "ada@example.com\n", strings.Repeat("a", 250) + "@x.io"} {
		if got := InviteEmailProblem(bad); got == "" {
			t.Errorf("%q: no problem, want one", bad)
		}
	}
}

func TestEmailMaySignInFollowsTheDomainRule(t *testing.T) {
	s := &Service{cfg: Config{AllowedEmailDomains: []string{"example.com"}}}
	if !s.EmailMaySignIn("Ada@Example.com") || s.EmailMaySignIn("ada@other.io") {
		t.Error("the allowed domain rule is not applied")
	}
	s.cfg.AllowAnyEmailDomain = true
	if !s.EmailMaySignIn("ada@other.io") {
		t.Error("allow-any refused a domain")
	}
}

func TestSignInURLIsTheLoginPageOfThePublicURL(t *testing.T) {
	s := &Service{cfg: Config{PublicURL: mustURL(t, "https://observe.example.com/")}}
	if got := s.SignInURL(); got != "https://observe.example.com/login" {
		t.Errorf("SignInURL = %q", got)
	}
}
