package prompts

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	MaxNameRunes = 64
	MaxPerOwner  = 100
)

const (
	messageInvalidName     = "name must be 1 to 64 letters, numbers, spaces, - or _, starting and ending with a letter or number"
	messageInvalidBody     = "body must be 1 to 4000 characters without control or bidirectional override characters"
	messageOwnNameTaken    = "you already have a prompt with that name"
	messageSharedNameTaken = "another shared prompt has that name"
	messageTooMany         = "a person can save at most 100 prompts"
	messageNotOwner        = "only the owner or an admin can change this prompt"
)

var namePattern = regexp.MustCompile(`^[\p{L}\p{N}](?:[\p{L}\p{N} _-]*[\p{L}\p{N}])?$`)

func NormalizeName(raw string) (string, bool) {
	name := strings.Join(strings.Fields(raw), " ")
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxNameRunes || !namePattern.MatchString(name) {
		return "", false
	}
	return name, true
}

func CleanBody(raw string) (string, bool) {
	return revisions.CleanPrompt(raw)
}

func CanEdit(caller auth.Principal, ownerID string, shared bool) bool {
	if caller.Kind != auth.KindUser || !caller.Role.AtLeast(auth.RoleEditor) {
		return false
	}
	return caller.ID == ownerID || (shared && caller.Role == auth.RoleAdmin)
}

type Patch struct {
	Name   *string `json:"name"`
	Body   *string `json:"body"`
	Shared *bool   `json:"shared"`
}

func (p Patch) Empty() bool {
	return p.Name == nil && p.Body == nil && p.Shared == nil
}

func (p Patch) Apply(name, body string, shared bool) (string, string, bool) {
	if p.Name != nil {
		name = *p.Name
	}
	if p.Body != nil {
		body = *p.Body
	}
	if p.Shared != nil {
		shared = *p.Shared
	}
	return name, body, shared
}
