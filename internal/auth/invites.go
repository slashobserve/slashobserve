package auth

import (
	"strings"
)

func InviteEmailProblem(email string) string {
	at := strings.LastIndex(email, "@")
	switch {
	case emailFailure(email) != nil, strings.ContainsAny(email, " \t"):
		return "email must be one address of at most 254 bytes without spaces or control characters"
	case at < 1 || at == len(email)-1:
		return "email must look like name@example.com"
	}
	return ""
}

func (s *Service) EmailMaySignIn(email string) bool {
	return s.cfg.AllowAnyEmailDomain || emailAllowed(email, true, s.cfg.AllowedEmailDomains)
}

func (s *Service) SignInURL() string {
	return s.cfg.PublicURL.JoinPath("login").String()
}
