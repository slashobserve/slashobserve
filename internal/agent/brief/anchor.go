package brief

import (
	"fmt"
	"strings"

	"github.com/slashobserve/slashobserve/internal/anchor"
)

const maxAnchorRunes = 500

var tagSafe = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

type Pointer struct {
	Anchor        *anchor.Anchor
	BlockID       string
	ObservationID string
}

func anchorNote(p Pointer) string {
	if p.Anchor == nil {
		return ""
	}
	what := strings.Join(strings.Fields(p.Anchor.Describe()), " ")
	if runes := []rune(what); len(runes) > maxAnchorRunes {
		what = string(runes[:maxAnchorRunes]) + "..."
	}
	what = tagSafe.Replace(what)
	where := ""
	if p.BlockID != "" {
		where = " on block " + p.BlockID
		if p.ObservationID != "" {
			where += " of observation " + p.ObservationID
		}
	}
	return fmt.Sprintf("The request points at %q%s.", what, where)
}
