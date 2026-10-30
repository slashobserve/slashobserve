package assets

import (
	"strings"
	"testing"
)

func TestSniffImageMatchesTheDeclaredType(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 8)
	jpeg := "\xff\xd8\xff\xe0" + strings.Repeat("\x00", 8)
	webp := "RIFF\x00\x00\x00\x00WEBPVP8 "
	avif := "\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avifmif1"
	avis := "\x00\x00\x00\x1cftypavis\x00\x00\x00\x00"
	svg := `<?xml version="1.0"?><!-- c --><svg xmlns="http://www.w3.org/2000/svg" width="1"/>`
	cases := []struct {
		name      string
		body      string
		mediaType string
		want      bool
	}{
		{"png", png, "image/png", true},
		{"png with a short signature", "\x89PNG", "image/png", false},
		{"html declared as png", "<html><body>hi</body></html>", "image/png", false},
		{"jpeg", jpeg, "image/jpeg", true},
		{"png declared as jpeg", png, "image/jpeg", false},
		{"gif87a", "GIF87a\x01\x00", "image/gif", true},
		{"gif89a", "GIF89a\x01\x00", "image/gif", true},
		{"gif with another version", "GIF90a", "image/gif", false},
		{"webp", webp, "image/webp", true},
		{"riff that is not webp", "RIFF\x00\x00\x00\x00WAVEfmt ", "image/webp", false},
		{"webp too short", "RIFF", "image/webp", false},
		{"avif", avif, "image/avif", true},
		{"avis", avis, "image/avif", true},
		{"ftyp for another brand", "\x00\x00\x00\x1cftypmp42\x00\x00\x00\x00isom", "image/avif", false},
		{"avif brand past the first 32 bytes", "\x00\x00\x00\x1cftypmp42" + strings.Repeat("\x00", 30) + "avif", "image/avif", false},
		{"svg with a prolog", svg, "image/svg+xml", true},
		{"svg with a script", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, "image/svg+xml", true},
		{"xml that is not svg", `<?xml version="1.0"?><rss><channel/></rss>`, "image/svg+xml", false},
		{"html declared as svg", `<html><body/></html>`, "image/svg+xml", false},
		{"text declared as svg", "not xml at all", "image/svg+xml", false},
		{"empty svg", "", "image/svg+xml", false},
		{"unlisted type", png, "image/bmp", false},
		{"empty type", png, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sniffImage([]byte(tc.body), tc.mediaType); got != tc.want {
				t.Errorf("sniffImage(%q) = %v, want %v", tc.mediaType, got, tc.want)
			}
		})
	}
}

func TestValidGeoJSONKeepsLargeIntegersAndRefusesOtherShapes(t *testing.T) {
	collection := `{"type":"FeatureCollection","features":[{"type":"Feature","id":9007199254740993,"properties":{"n":12345678901234567890},"geometry":{"type":"Point","coordinates":[1.5,2]}}]}`
	out, ok := validGeoJSON([]byte(collection))
	if !ok {
		t.Fatal("a FeatureCollection was refused")
	}
	for _, want := range []string{`"id":9007199254740993`, `"n":12345678901234567890`, `"coordinates":[1.5,2]`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output %s lost %s", out, want)
		}
	}

	for _, kind := range geoJSONTypes {
		if _, ok := validGeoJSON([]byte(`{"type":"` + kind + `"}`)); !ok {
			t.Errorf("type %s was refused", kind)
		}
	}

	refused := map[string]string{
		"array":             `[{"type":"Feature"}]`,
		"unknown type":      `{"type":"Nope"}`,
		"type not a string": `{"type":3}`,
		"no type":           `{"features":[]}`,
		"scalar":            `"FeatureCollection"`,
		"truncated":         `{"type":"Feature"`,
		"trailing data":     `{"type":"Feature"} {"type":"Feature"}`,
		"html":              `<html></html>`,
		"empty":             ``,
	}
	for name, body := range refused {
		t.Run(name, func(t *testing.T) {
			if _, ok := validGeoJSON([]byte(body)); ok {
				t.Errorf("%s was accepted", body)
			}
		})
	}
}
