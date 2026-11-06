package llm

import (
	"fmt"
	"unicode/utf8"
)

type Weighed struct {
	Assistant bool
	Bytes     int
}

func TailStart(messages []Weighed, tailTurns, tailBytes int) int {
	last := lastAssistant(messages, len(messages))
	if last < 0 {
		return len(messages)
	}
	start, kept, size := last, 1, sumBytes(messages[last:])
	for kept < tailTurns {
		previous := lastAssistant(messages, start)
		if previous < 0 {
			break
		}
		turn := sumBytes(messages[previous:start])
		if size+turn > tailBytes {
			break
		}
		start, kept, size = previous, kept+1, size+turn
	}
	return start
}

func lastAssistant(messages []Weighed, before int) int {
	for i := before - 1; i >= 0; i-- {
		if messages[i].Assistant {
			return i
		}
	}
	return -1
}

func sumBytes(messages []Weighed) int {
	total := 0
	for _, m := range messages {
		total += m.Bytes
	}
	return total
}

func CutResult(content string, n int) string {
	if n <= 0 || len(content) <= n {
		return content
	}
	end := n
	for end > 0 && !utf8.RuneStart(content[end]) {
		end--
	}
	return fmt.Sprintf("%s\n[cut from %d bytes; call the tool again for the full result]", content[:end], len(content))
}

func KeptBytes(n, limit int) int {
	if limit <= 0 {
		return n
	}
	return min(n, limit)
}

const PrunedResult = "[earlier result left out to keep the conversation short; call the tool again if you need it]"

type PruneMessage struct {
	Assistant bool
	Results   []PruneResult
}

type PruneResult struct {
	ID    string
	Bytes int
}

type Pruner struct {
	cfg      Pruning
	prunable map[string]bool
	boundary int
}

func NewPruner(cfg Pruning) *Pruner {
	return &Pruner{cfg: cfg, prunable: map[string]bool{}}
}

func (p *Pruner) Note(results []ToolResult) {
	for _, r := range results {
		if r.Prunable {
			p.prunable[r.CallID] = true
		}
	}
}

func (p *Pruner) Reset() { p.boundary = 0 }

func (p *Pruner) Prunable(id string) bool { return p.prunable[id] }

func (p *Pruner) Boundary(messages []PruneMessage) int {
	if p.cfg.StepBytes <= 0 || p.cfg.KeepTurns <= 0 {
		return 0
	}
	cut, ok := keptFrom(messages, p.cfg.KeepTurns)
	if !ok {
		return p.boundary
	}
	candidates := 0
	for _, m := range messages[min(p.boundary, cut):cut] {
		for _, r := range m.Results {
			if p.prunable[r.ID] {
				candidates += r.Bytes
			}
		}
	}
	if candidates >= p.cfg.StepBytes {
		p.boundary = cut
	}
	return p.boundary
}

func keptFrom(messages []PruneMessage, keepTurns int) (int, bool) {
	seen := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Assistant {
			if seen++; seen == keepTurns {
				return i, true
			}
		}
	}
	return 0, false
}
