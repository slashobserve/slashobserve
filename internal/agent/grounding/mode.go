package grounding

import "fmt"

type Mode string

const (
	Off     Mode = "off"
	Report  Mode = "report"
	Enforce Mode = "enforce"
)

func ParseMode(raw string) (Mode, error) {
	switch mode := Mode(raw); mode {
	case "":
		return Enforce, nil
	case Off, Enforce:
		return mode, nil
	case Report:
		return Off, fmt.Errorf("report is no longer a mode, because the agent sends an ungrounded finding back or stores it as it is; use enforce, the default, or off")
	}
	return Off, fmt.Errorf("must be off or enforce, got %q", raw)
}
