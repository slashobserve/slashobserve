package grafana

import (
	"fmt"
	"strings"
	"testing"
)

func TestDetectFolderTitle(t *testing.T) {
	inner := `{"title":"T","panels":[` + panelJSON(1, 0, 0, 12, 4, "") + `]}`
	api := func(meta string) string {
		return fmt.Sprintf(`{"dashboard":%s,%s}`, inner, meta)
	}
	classicWithMeta := `{"title":"T","meta":{"folderTitle":"Checkout"},"panels":[` + panelJSON(1, 0, 0, 12, 4, "") + `]}`

	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"api wrapper folder", api(`"meta":{"folderTitle":"Checkout"}`), "Checkout"},
		{"surrounding spaces trimmed", api(`"meta":{"folderTitle":" Checkout "}`), "Checkout"},
		{"api wrapper without meta", fmt.Sprintf(`{"dashboard":%s}`, inner), ""},
		{"meta is not an object", api(`"meta":"Checkout"`), ""},
		{"folder title is not a string", api(`"meta":{"folderTitle":7}`), ""},
		{"blank folder title", api(`"meta":{"folderTitle":"   "}`), ""},
		{"100 runes kept", api(`"meta":{"folderTitle":"` + strings.Repeat("é", 100) + `"}`), strings.Repeat("é", 100)},
		{"101 runes dropped", api(`"meta":{"folderTitle":"` + strings.Repeat("é", 101) + `"}`), ""},
		{"newline dropped", api(`"meta":{"folderTitle":"Check\nout"}`), ""},
		{"format character dropped", api("\"meta\":{\"folderTitle\":\"Check\\u200bout\"}"), ""},
		{"classic format ignores meta", classicWithMeta, ""},
		{"root folder keeps its name", api(`"meta":{"folderTitle":"General"}`), "General"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := Detect([]byte(tt.doc))
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if summary.FolderTitle != tt.want {
				t.Errorf("FolderTitle = %q, want %q", summary.FolderTitle, tt.want)
			}
		})
	}
}
