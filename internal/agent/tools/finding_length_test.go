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
