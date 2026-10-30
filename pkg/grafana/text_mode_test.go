package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func textPanel(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, `{"title":"T","schemaVersion":39,"panels":[{`+panelFrame+`,"type":"text","options":`+options+`}]}`)
}

func TestTextHTMLModeIsImportedWithoutReports(t *testing.T) {
	panel, line := textPanel(t, `{"mode":"html","content":"<b>Hi</b>"}`)
	if panel.Mode != "html" || line.Status != statusImported {
		t.Fatalf("mode %q status %s paths %v", panel.Mode, line.Status, line.Paths)
	}
}

func TestTextCodeModeKeepsLanguageAndLineNumbers(t *testing.T) {
	panel, line := textPanel(t, `{"mode":"code","content":"{}","code":{"language":"json","showLineNumbers":true,"showMiniMap":true}}`)
	if panel.Mode != "code" || panel.Code == nil || panel.Code.Language != "json" || panel.Code.LineNumbers == nil || !*panel.Code.LineNumbers {
		t.Fatalf("mode %q code %+v", panel.Mode, panel.Code)
	}
	if line.Status != statusImported {
		t.Fatalf("status %s paths %v", line.Status, line.Paths)
	}
}

func TestTextCodeModeWithTheDefaultLanguageHasNoCodeBlock(t *testing.T) {
	panel, line := textPanel(t, `{"mode":"code","content":"x","code":{"language":"plaintext","showLineNumbers":false,"showMiniMap":false}}`)
	if panel.Mode != "code" || panel.Code != nil || line.Status != statusImported {
		t.Fatalf("mode %q code %+v status %s paths %v", panel.Mode, panel.Code, line.Status, line.Paths)
	}
}

func TestTextCodeModeReportsALanguageTheRendererDoesNotKnow(t *testing.T) {
	panel, line := textPanel(t, `{"mode":"code","content":"x","code":{"language":"rust"}}`)
	if panel.Mode != "code" || panel.Code != nil {
		t.Fatalf("mode %q code %+v", panel.Mode, panel.Code)
	}
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.code.language") {
		t.Fatalf("status %s paths %v", line.Status, line.Paths)
	}
}

func TestTextCodeOptionsAreIgnoredOutsideCodeMode(t *testing.T) {
	panel, line := textPanel(t, `{"mode":"markdown","content":"x","code":{"language":"go","showLineNumbers":true}}`)
	if panel.Mode != "" || panel.Code != nil || line.Status != statusImported {
		t.Fatalf("mode %q code %+v status %s paths %v", panel.Mode, panel.Code, line.Status, line.Paths)
	}
}
