package prompts

import _ "embed"

//go:embed spec_guide.md
var specGuide string

func SpecGuide() string { return specGuide }
