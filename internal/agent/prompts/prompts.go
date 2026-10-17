package prompts

import _ "embed"

//go:embed spec_guide.md
var specGuide string

//go:embed system.md
var system string

func SpecGuide() string { return specGuide }

func System() string { return system }
