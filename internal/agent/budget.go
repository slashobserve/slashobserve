package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const (
	maxRejectionsInRow       = 3
	maxCutOffsInRow          = 2
	maxIdenticalReads        = 3
	maxCallsBeforeSave       = 6
	maxCallsBeforeProposal   = 12
	maxStalledTurns          = 2
	maxChangesPerPanel       = 4
	maxChurnedSavesInRow     = 3
	defaultSessionRenewBytes = 256 << 10

	compactTailTurns   = 4
	compactTailBytes   = 32 << 10
	compactResultBytes = 2 << 10

	pruneKeepTurns = 4
	pruneStepBytes = 64 << 10
)

const settingsSubject = ""

var ErrNoProgress = errors.New("the agent stopped making progress")

type guard struct {
	message string
	refused string
	stopped string
}

var (
	guardSaveNow = guard{
		message: fmt.Sprintf("%d model calls without a save; only a save can run now. Save the panel you have checked, or call save_finding if the request only asks a question; reads in the same call as a save still run.", maxCallsBeforeSave),
		refused: fmt.Sprintf("%d model calls passed without a save", maxCallsBeforeSave),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a save", maxCallsBeforeSave),
	}
	guardFindingNow = guard{
		message: fmt.Sprintf("%d model calls without a finding; only save_finding can run now. Save what you found and the evidence you checked.", maxCallsBeforeProposal),
		refused: fmt.Sprintf("%d model calls passed without a finding", maxCallsBeforeProposal),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a finding", maxCallsBeforeProposal),
	}
	guardProposeNow = guard{
		message: fmt.Sprintf("%d model calls without a proposal; only a proposal can run now. Propose the panel you have checked.", maxCallsBeforeProposal),
		refused: fmt.Sprintf("%d model calls passed without a proposal", maxCallsBeforeProposal),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a proposal", maxCallsBeforeProposal),
	}
)

var (
	guardRepeatedRead = guard{
		message: fmt.Sprintf("this exact call already ran %d times; use its earlier result, change its input, or save", maxIdenticalReads-1),
		refused: fmt.Sprintf("the same read was asked %d times", maxIdenticalReads),
		stopped: fmt.Sprintf("Stopped: the same read was asked %d times", maxIdenticalReads),
	}
	guardRepeatedSave = guard{
		message: "this save is identical to an earlier one and changes nothing; save the next panel, or set finish: true when every planned panel is built",
		refused: "the same save was sent again",
		stopped: "Stopped: the same save was sent again and again",
	}
)

const (
	messageReplacement = "This save deleted a panel and added another, which is not progress. Keep the panels you have, save the next panel, or set finish: true when every planned panel is built."
	messageReplayed    = "This is the result of the same earlier call. A third identical call is refused."

	messageSaveNext    = "Save the next panel, or call save_finding if the request only asks a question, in your next call. From then on, reads run only in the same call as a save."
	messageProposeNext = "Propose the panel in your next call. From then on, reads run only in the same call as a proposal."
	messageFindingNext = "One more read at most, then call save_finding."
)

type saveWindow struct {
	calls    int
	stalled  int
	diagnose bool
}

func windowLimit(mode tools.Mode) int {
	if mode == tools.ModeSuggest || mode == tools.ModeInvestigate {
		return maxCallsBeforeProposal
	}
	return maxCallsBeforeSave
}

func readRefusal(mode tools.Mode, w saveWindow, hasSave bool) guard {
	if hasSave || w.diagnose || w.calls < windowLimit(mode) {
		return guard{}
	}
	switch mode {
	case tools.ModeSuggest:
		return guardProposeNow
	case tools.ModeInvestigate:
		return guardFindingNow
	}
	return guardSaveNow
}

func nudgeFor(mode tools.Mode, w saveWindow) string {
	if w.calls != windowLimit(mode)-1 {
		return ""
	}
	switch mode {
	case tools.ModeSuggest:
		return messageProposeNext
	case tools.ModeInvestigate:
		return messageFindingNext
	}
	return messageSaveNext
}

type churn struct {
	changes map[string]int
	inRow   int
}

func (c *churn) note(edits tools.Edits) (note string, stop bool) {
	if c.changes == nil {
		c.changes = map[string]int{}
	}
	subjects := slices.Clone(edits.Panels)
	if edits.Settings {
		subjects = append(subjects, settingsSubject)
	}
	replaced := len(edits.Deleted) > 0
	progressed := false
	for _, id := range subjects {
		c.changes[id]++
		isReplacement := replaced && slices.Contains(edits.Added, id)
		progressed = progressed || c.changes[id] <= maxChangesPerPanel && !isReplacement
	}
	if progressed {
		c.inRow = 0
		return "", false
	}
	c.inRow++
	if replaced && len(edits.Added) > 0 {
		return messageReplacement, c.inRow > maxChurnedSavesInRow
	}
	subject := "the build"
	if len(subjects) > 0 {
		subject = subjects[0]
	}
	if subject == settingsSubject {
		subject = "the dashboard settings"
	}
	note = fmt.Sprintf("This save changed no panel that still needs work. Leave %s as it is, save the next panel, or set finish: true when every planned panel is built.", subject)
	return note, c.inRow > maxChurnedSavesInRow
}

func callKey(name string, input json.RawMessage) string {
	var compact bytes.Buffer
	body := []byte(input)
	if json.Compact(&compact, input) == nil {
		body = compact.Bytes()
	}
	sum := sha256.Sum256(body)
	return name + ":" + hex.EncodeToString(sum[:])
}

type cachedRead struct {
	count   int
	content []byte
}

type readDecision int

const (
	readDispatch readDecision = iota
	readReplay
	readRefuse
)

type guards struct {
	saves map[string]bool
	reads map[string]*cachedRead
}

func (g *guards) noteSave(key string) (repeated bool) {
	if g.saves[key] {
		return true
	}
	if g.saves == nil {
		g.saves = map[string]bool{}
	}
	g.saves[key] = true
	return false
}

func (g *guards) noteRead(key string) (readDecision, []byte) {
	if g.reads == nil {
		g.reads = map[string]*cachedRead{}
	}
	entry := g.reads[key]
	if entry == nil {
		entry = &cachedRead{}
		g.reads[key] = entry
	}
	entry.count++
	switch {
	case entry.count >= maxIdenticalReads:
		return readRefuse, nil
	case entry.content != nil:
		return readReplay, entry.content
	}
	return readDispatch, nil
}

func (g *guards) rememberRead(key, tool string, content []byte, isError bool) {
	if isError || phaseFor(tool, false) == phaseQuery {
		return
	}
	if entry := g.reads[key]; entry != nil {
		entry.content = content
	}
}

func (g *guards) resetReads() {
	g.reads = nil
}

func (g *guards) resetAll() {
	g.reads, g.saves = nil, nil
}
