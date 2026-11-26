package tools

import (
	"strings"
	"testing"
)

func TestSaveFindingTextLetsANamedLengthDropTheUncheckedNote(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if tool.Name != nameSaveFinding {
			continue
		}
		offered := tool.ForAgent()
		text := offered.Description + string(offered.InputSchema)
		for _, want := range []string{
			"say what you did not check unless the request names a length or a form",
			"then the evidence unless the request names a length or form",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("save_finding text lacks %q", want)
			}
		}
		return
	}
	t.Fatal("save_finding is not offered")
}

func TestTheLengthRejectionNamesTheLimitAndAsksForAShorterText(t *testing.T) {
	_, res, ok := findingMarkdown(strings.Repeat("word ", 413), 0, nil)
	if ok || errorCode(t, res) != CodeFindingTooLong {
		t.Fatalf("413 words: ok %v, res %#v", ok, res.Content)
	}
	message := res.Content.(errorBody).Message
	for _, want := range []string{"413 words", "limit is 400", "shorten the text by 13 words", "keeping every figure the question asks for"} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q lacks %q", message, want)
		}
	}
	if strings.Contains(message, "panel") {
		t.Errorf("message %q points at panels", message)
	}
}
