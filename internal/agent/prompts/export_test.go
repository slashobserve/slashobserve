package prompts

var StripSchema = stripSchema

func StripAnnotations(raw []byte) ([]byte, error) { return stripper{keepMarked: true}.strip(raw) }
