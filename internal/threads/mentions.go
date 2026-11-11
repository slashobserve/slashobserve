package threads

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	MaxMentions  = 20
	AgentObserve = "observe"

	agentHandle  = "@" + AgentObserve
	maxNameRunes = 256
	mentionUser  = "user"
	mentionAgent = "agent"
)

var userToken = regexp.MustCompile(`@\[([^\]\n]*)\]\(user:([0-9a-fA-F-]{36})\)`)

type Mention struct {
	UserID pgtype.UUID
	Agent  string
	Name   string
}

func (m Mention) Kind() string {
	if m.Agent != "" {
		return mentionAgent
	}
	return mentionUser
}

func (m Mention) RefID() string {
	if m.Agent != "" {
		return m.Agent
	}
	return m.UserID.String()
}

func ParseMentions(body string) ([]Mention, error) {
	var mentions []Mention
	seen := map[string]bool{}
	add := func(m Mention) {
		if key := m.Kind() + ":" + m.RefID(); !seen[key] {
			seen[key] = true
			mentions = append(mentions, m)
		}
	}
	masked := userToken.ReplaceAllStringFunc(body, func(token string) string {
		parts := userToken.FindStringSubmatch(token)
		id, err := db.ParseUUID(parts[2])
		if err != nil {
			return token
		}
		add(Mention{UserID: id, Name: parts[1]})
		return strings.Repeat(" ", len(token))
	})
	if hasAgentMention(masked) {
		add(Mention{Agent: AgentObserve, Name: AgentObserve})
	}
	if len(mentions) > MaxMentions {
		return nil, &InvalidInputError{Code: CodeTooManyMentions, Message: "a comment can mention at most 20 people or agents"}
	}
	return mentions, nil
}

func hasAgentMention(text string) bool {
	for offset := 0; ; {
		at := strings.Index(text[offset:], agentHandle)
		if at < 0 {
			return false
		}
		start := offset + at
		end := start + len(agentHandle)
		if wordBoundaryBefore(text[:start]) && wordBoundaryAfter(text[end:]) {
			return true
		}
		offset = end
	}
}

func wordBoundaryBefore(prefix string) bool {
	r, _ := utf8.DecodeLastRuneInString(prefix)
	return prefix == "" || !isHandleRune(r)
}

func wordBoundaryAfter(suffix string) bool {
	r, _ := utf8.DecodeRuneInString(suffix)
	return suffix == "" || !isHandleRune(r)
}

func isHandleRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

func capName(name string) string {
	if r := []rune(name); len(r) > maxNameRunes {
		return string(r[:maxNameRunes])
	}
	return name
}
