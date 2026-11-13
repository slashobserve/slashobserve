package threads

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	MaxMentions  = 20
	AgentObserve = "observe"

	agentHandle           = "@" + AgentObserve
	maxNameRunes          = 256
	mentionUser           = "user"
	mentionAgent          = "agent"
	mentionGrant          = "grant"
	mentionServiceAccount = "service_account"
)

var mentionToken = regexp.MustCompile(`@\[([^\]\n]*)\]\((user|grant|service_account):([0-9a-fA-F-]{36})\)`)

type Mention struct {
	UserID           pgtype.UUID
	Agent            string
	GrantID          pgtype.UUID
	ServiceAccountID pgtype.UUID
	Name             string
}

func (m Mention) Kind() string {
	switch {
	case m.Agent != "":
		return mentionAgent
	case m.GrantID.Valid:
		return mentionGrant
	case m.ServiceAccountID.Valid:
		return mentionServiceAccount
	}
	return mentionUser
}

func (m Mention) RefID() string {
	switch {
	case m.Agent != "":
		return m.Agent
	case m.GrantID.Valid:
		return m.GrantID.String()
	case m.ServiceAccountID.Valid:
		return m.ServiceAccountID.String()
	}
	return m.UserID.String()
}

func (m Mention) isAgent() bool {
	return m.Kind() != mentionUser
}

type Mentions []Mention

func (ms Mentions) Agents() []Mention {
	var agents []Mention
	for _, m := range ms {
		if m.isAgent() {
			agents = append(agents, m)
		}
	}
	return agents
}

type tokenAt struct {
	start, end int
	mention    Mention
}

func tokensOf(body string) []tokenAt {
	var tokens []tokenAt
	for _, loc := range mentionToken.FindAllStringSubmatchIndex(body, -1) {
		id, err := db.ParseUUID(body[loc[6]:loc[7]])
		if err != nil {
			continue
		}
		m := Mention{Name: body[loc[2]:loc[3]]}
		switch body[loc[4]:loc[5]] {
		case mentionGrant:
			m.GrantID = id
		case mentionServiceAccount:
			m.ServiceAccountID = id
		default:
			m.UserID = id
		}
		tokens = append(tokens, tokenAt{start: loc[0], end: loc[1], mention: m})
	}
	return tokens
}

func maskTokens(body string, tokens []tokenAt) string {
	masked := []byte(body)
	for _, t := range tokens {
		for i := t.start; i < t.end; i++ {
			masked[i] = ' '
		}
	}
	return string(masked)
}

func ParseMentions(body string) (Mentions, error) {
	var mentions Mentions
	seen := map[string]bool{}
	add := func(m Mention) {
		if key := m.Kind() + ":" + m.RefID(); !seen[key] {
			seen[key] = true
			mentions = append(mentions, m)
		}
	}
	tokens := tokensOf(body)
	for _, t := range tokens {
		add(t.mention)
	}
	if len(handleSpans(maskTokens(body, tokens))) > 0 {
		add(Mention{Agent: AgentObserve, Name: AgentObserve})
	}
	if len(mentions) > MaxMentions {
		return nil, &InvalidInputError{Code: CodeTooManyMentions, Message: "a comment can mention at most 20 people or agents"}
	}
	return mentions, nil
}

func withoutMention(body string, target Mention) string {
	tokens := tokensOf(body)
	var cuts [][2]int
	if target.Kind() == mentionAgent {
		cuts = handleSpans(maskTokens(body, tokens))
	}
	for _, t := range tokens {
		if t.mention.Kind() == target.Kind() && t.mention.RefID() == target.RefID() {
			cuts = append(cuts, [2]int{t.start, t.end})
		}
	}
	slices.SortFunc(cuts, func(a, b [2]int) int { return cmp.Compare(a[0], b[0]) })
	var out strings.Builder
	at := 0
	for _, c := range cuts {
		out.WriteString(body[at:c[0]])
		at = c[1]
	}
	out.WriteString(body[at:])
	return strings.TrimSpace(out.String())
}

func handleSpans(text string) [][2]int {
	var spans [][2]int
	for offset := 0; ; {
		at := strings.Index(text[offset:], agentHandle)
		if at < 0 {
			return spans
		}
		start := offset + at
		end := start + len(agentHandle)
		if wordBoundaryBefore(text[:start]) && wordBoundaryAfter(text[end:]) {
			spans = append(spans, [2]int{start, end})
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
