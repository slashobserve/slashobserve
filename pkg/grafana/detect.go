package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type source struct {
	format Format
	bytes  int
	dash   map[string]any
	nodes  []*panelNode
	clones []string
	legacy bool

	folderTitle string
}

const maxFolderTitleRunes = 100

func Detect(raw []byte) (Summary, error) {
	src, err := load(raw)
	if err != nil {
		return Summary{}, err
	}
	return src.summary(), nil
}

func (s *source) summary() Summary {
	summary := Summary{Format: s.format, Bytes: s.bytes}
	summary.Title = asString(s.dash["title"])
	summary.UID = asString(s.dash["uid"])
	summary.FolderTitle = s.folderTitle
	summary.SchemaVersion, _ = asInt(s.dash["schemaVersion"])
	s.walk(func(n *panelNode) {
		if n.row {
			summary.Rows++
		} else {
			summary.Panels++
		}
	})
	if templating, ok := asObject(s.dash["templating"]); ok {
		list, _ := asArray(templating["list"])
		summary.Variables = len(list)
	}
	return summary
}

func (s *source) walk(visit func(*panelNode)) {
	for _, n := range s.nodes {
		visit(n)
		for _, child := range n.children {
			visit(child)
		}
	}
}

func load(raw []byte) (*source, error) {
	doc, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	src := &source{format: formatClassic, bytes: len(raw), dash: doc}
	if inner, ok := asObject(doc["dashboard"]); ok && !hasPanelsOrRows(doc) {
		src.format = formatAPI
		src.dash = inner
		if meta, ok := asObject(doc["meta"]); ok {
			src.folderTitle = cleanFolderTitle(asString(meta["folderTitle"]))
		}
	}
	if isV2(src.dash) {
		return nil, inputErrorf(codeUnsupportedFormat, "Grafana v2 dashboards are not supported yet; export the classic JSON model")
	}
	if _, ok := src.dash["__inputs"]; ok && src.format == formatClassic {
		src.format = formatExport
	}
	if !hasPanelsOrRows(src.dash) {
		return nil, inputErrorf(codeNotADashboard, "the JSON is not a Grafana dashboard: it has no panels or rows")
	}
	inlineLibraryPanels(src.dash)
	top, legacy := buildNodes(src.dash)
	src.legacy = legacy
	src.nodes, src.clones = arrange(top)
	total := 0
	src.walk(func(*panelNode) { total++ })
	if total == 0 {
		return nil, inputErrorf(codeNotADashboard, "the dashboard has no panels")
	}
	if total > spec.MaxPanels {
		return nil, inputErrorf(codeTooManyPanels, "the dashboard has %d panels including rows; a SlashObserve dashboard holds at most %d", total, spec.MaxPanels)
	}
	return src, nil
}

func cleanFolderTitle(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > maxFolderTitleRunes {
		return ""
	}
	for _, r := range title {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ""
		}
	}
	return title
}

func hasPanelsOrRows(d map[string]any) bool {
	_, panels := asArray(d["panels"])
	_, rows := asArray(d["rows"])
	return panels || rows
}

func isV2(d map[string]any) bool {
	if strings.HasPrefix(asString(d["apiVersion"]), "dashboard.grafana.app/") {
		return true
	}
	if s, ok := asObject(d["spec"]); ok {
		_, elements := asObject(s["elements"])
		return elements
	}
	return false
}

func decodeObject(raw []byte) (map[string]any, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		return nil, inputErrorf(codeInvalidJSON, "the input is not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, inputErrorf(codeInvalidJSON, "the input is empty")
		}
		return nil, inputErrorf(codeInvalidJSON, "the input is not valid JSON: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, inputErrorf(codeInvalidJSON, "the input has data after the JSON value")
	}
	obj, ok := asObject(doc)
	if !ok {
		return nil, inputErrorf(codeInvalidJSON, "the input must be one JSON object")
	}
	return obj, nil
}
