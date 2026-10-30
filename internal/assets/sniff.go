package assets

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"slices"
)

var (
	pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	jpegStart    = []byte{0xff, 0xd8, 0xff}

	geoJSONTypes = []string{
		"FeatureCollection", "Feature", "GeometryCollection",
		"Point", "MultiPoint", "LineString", "MultiLineString", "Polygon", "MultiPolygon",
	}
)

const avifProbeBytes = 32

func sniffImage(body []byte, mediaType string) bool {
	switch mediaType {
	case "image/png":
		return bytes.HasPrefix(body, pngSignature)
	case "image/jpeg":
		return bytes.HasPrefix(body, jpegStart)
	case "image/gif":
		return bytes.HasPrefix(body, []byte("GIF87a")) || bytes.HasPrefix(body, []byte("GIF89a"))
	case "image/webp":
		return len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP"))
	case "image/avif":
		return sniffAVIF(body)
	case "image/svg+xml":
		return sniffSVG(body)
	}
	return false
}

func sniffAVIF(body []byte) bool {
	if len(body) < 12 || !bytes.Equal(body[4:8], []byte("ftyp")) {
		return false
	}
	head := body[:min(len(body), avifProbeBytes)]
	return bytes.Contains(head, []byte("avif")) || bytes.Contains(head, []byte("avis"))
}

func sniffSVG(body []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == "svg"
		}
	}
}

func validGeoJSON(body []byte) ([]byte, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var object map[string]any
	if err := dec.Decode(&object); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	kind, _ := object["type"].(string)
	if !slices.Contains(geoJSONTypes, kind) {
		return nil, false
	}
	out, err := json.Marshal(object)
	if err != nil {
		return nil, false
	}
	return out, true
}
