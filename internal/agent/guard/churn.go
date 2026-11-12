package guard

import (
	"fmt"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const settingsSubject = ""

func (s *State) noteChurn(edits tools.Edits) (note string, stop bool) {
	if s.Churn == nil {
		s.Churn = map[string]int{}
	}
	subjects := slices.Clone(edits.Panels)
	if edits.Settings {
		subjects = append(subjects, settingsSubject)
	}
	replaced := len(edits.Deleted) > 0
	progressed := false
	for _, id := range subjects {
		s.Churn[id]++
		isReplacement := replaced && slices.Contains(edits.Added, id)
		progressed = progressed || s.Churn[id] <= maxChangesPerPanel && !isReplacement
	}
	if progressed {
		s.ChurnedInRow = 0
		return "", false
	}
	s.ChurnedInRow++
	if replaced && len(edits.Added) > 0 {
		return messageReplacement, s.ChurnedInRow > maxChurnedSavesInRow
	}
	subject := "the build"
	if len(subjects) > 0 {
		subject = subjects[0]
	}
	if subject == settingsSubject {
		subject = "the dashboard settings"
	}
	note = fmt.Sprintf("This save changed no panel that still needs work. Leave %s as it is, save the next panel, or set finish: true when every planned panel is built.", subject)
	return note, s.ChurnedInRow > maxChurnedSavesInRow
}
