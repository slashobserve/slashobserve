package skill

import (
	"bytes"
	_ "embed"
	"strings"
)

const Name = "observe"

//go:embed observe/SKILL.md
var file []byte

var body = bodyOf(file)

func File() []byte {
	return bytes.Clone(file)
}

func Body() string {
	return body
}

func Prompt(ask string) string {
	ask = strings.TrimSpace(ask)
	if ask == "" {
		return body
	}
	return body + "\n\n## The ask\n\n" + ask
}

func bodyOf(file []byte) string {
	rest, ok := strings.CutPrefix(string(file), "---\n")
	if !ok {
		panic("skill file has no front matter")
	}
	_, after, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		panic("skill file front matter is not closed")
	}
	return strings.TrimLeft(after, "\n")
}
