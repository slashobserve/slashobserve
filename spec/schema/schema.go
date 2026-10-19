package schema

import _ "embed"

//go:embed observespec.v0.json
var ObserveSpecV0 []byte

//go:embed defaults.json
var Defaults []byte
