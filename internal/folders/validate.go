package folders

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	maxTitleRunes = 100
	maxEntries    = 50
	maxGroupBytes = 256
	maxEmailBytes = 320
)

type entryBody struct {
	UserID *string `json:"user_id"`
	Email  *string `json:"email"`
	Group  *string `json:"group"`
	Level  string  `json:"level"`
}

type entry struct {
	userID pgtype.UUID
	email  string
	group  string
	level  Level
}

func cleanTitle(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", errors.New("title must be valid UTF-8")
	}
	title := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(title); n < 1 || n > maxTitleRunes {
		return "", errors.New("title must be 1 to 100 characters")
	}
	if hasControl(title) {
		return "", errors.New("title must not contain control characters")
	}
	return title, nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

func parseEntries(bodies []entryBody) ([]entry, error) {
	if len(bodies) > maxEntries {
		return nil, errors.New("at most 50 entries are allowed")
	}
	seen := seenEntries{users: map[pgtype.UUID]bool{}, emails: map[string]bool{}, groups: map[string]bool{}}
	entries := make([]entry, 0, len(bodies))
	for _, b := range bodies {
		e, err := seen.parse(b)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

type seenEntries struct {
	users  map[pgtype.UUID]bool
	emails map[string]bool
	groups map[string]bool
}

func (s *seenEntries) parse(b entryBody) (entry, error) {
	level, ok := ParseLevel(b.Level)
	if !ok {
		return entry{}, errors.New("level must be view, edit or admin")
	}
	present := 0
	for _, p := range []*string{b.UserID, b.Email, b.Group} {
		if p != nil {
			present++
		}
	}
	if present != 1 {
		return entry{}, errors.New("each entry needs exactly one of user_id, email and group")
	}
	e := entry{level: level}
	var err error
	switch {
	case b.UserID != nil:
		e.userID, err = s.user(*b.UserID)
	case b.Email != nil:
		e.email, err = s.email(*b.Email)
	default:
		e.group, err = s.group(*b.Group)
	}
	return e, err
}

func (s *seenEntries) user(raw string) (pgtype.UUID, error) {
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, errors.New("user_id must be a UUID")
	}
	if s.users[id] {
		return pgtype.UUID{}, errors.New("a user can appear only once")
	}
	s.users[id] = true
	return id, nil
}

func (s *seenEntries) email(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailBytes || !utf8.ValidString(email) || hasControl(email) {
		return "", errors.New("email must be a valid address")
	}
	if s.emails[email] {
		return "", errors.New("a user can appear only once")
	}
	s.emails[email] = true
	return email, nil
}

func (s *seenEntries) group(group string) (string, error) {
	if len(group) < 1 || len(group) > maxGroupBytes || !utf8.ValidString(group) || hasControl(group) {
		return "", errors.New("group must be 1 to 256 bytes with no control characters")
	}
	if s.groups[group] {
		return "", errors.New("a group can appear only once")
	}
	s.groups[group] = true
	return group, nil
}
