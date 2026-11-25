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
		return Off, nil
	case Off, Report, Enforce:
		return mode, nil
	}
	return Off, fmt.Errorf("must be off, report or enforce, got %q", raw)
}
