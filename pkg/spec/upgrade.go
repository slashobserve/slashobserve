package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func Upgrade(data []byte) (out []byte, changed bool, err error) {
	doc, err := decodeDocument(data)
	if err != nil {
		return nil, false, err
	}
	steps := []func(map[string]any) (bool, error){renameTimeRange, upgradePanels, upgradeVariables}
	for _, step := range steps {
		stepChanged, err := step(doc)
		if err != nil {
			return nil, false, err
		}
		changed = changed || stepChanged
	}
	if !changed {
		return data, false, nil
	}
	out, err = json.Marshal(doc)
	if err != nil {
		return nil, false, fmt.Errorf("spec: upgrade: %w", err)
	}
	return out, true, nil
}

func decodeDocument(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, fmt.Errorf("spec: upgrade: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("spec: upgrade: unexpected data after the top-level JSON value")
	}
	doc, ok := tree.(map[string]any)
	if !ok {
		return nil, errors.New("spec: upgrade: document is not a JSON object")
	}
	return doc, nil
}

func renameTimeRange(doc map[string]any) (bool, error) {
	timeRange, ok := doc["timeRange"]
	if !ok {
		return false, nil
	}
	if _, both := doc["time"]; both {
		return false, errors.New(`spec: upgrade: document has both "timeRange" and "time"`)
	}
	doc["time"] = timeRange
	delete(doc, "timeRange")
	return true, nil
}

func upgradePanels(doc map[string]any) (changed bool, err error) {
	panels, _ := doc["panels"].([]any)
	for i, p := range panels {
		panel, ok := p.(map[string]any)
		if !ok {
			continue
		}
		panelChanged, err := upgradePanel(panel, i)
		if err != nil {
			return false, err
		}
		changed = changed || panelChanged
	}
	return changed, nil
}

func upgradeVariables(doc map[string]any) (changed bool, err error) {
	variables, _ := doc["variables"].([]any)
	for i, v := range variables {
		variable, ok := v.(map[string]any)
		if !ok {
			continue
		}
		variableChanged, err := flattenDatasource(variable, fmt.Sprintf("variables/%d", i))
		if err != nil {
			return false, err
		}
		changed = changed || variableChanged
	}
	return changed, nil
}

func upgradePanel(panel map[string]any, index int) (changed bool, err error) {
	path := fmt.Sprintf("panels/%d", index)
	changed, err = flattenDatasource(panel, path)
	if err != nil {
		return false, err
	}
	query, hasQuery := panel["query"]
	legend, hasLegend := panel["legend"]
	if !hasQuery {
		if hasLegend {
			return false, fmt.Errorf(`spec: upgrade: %s has "legend" without "query"`, path)
		}
		return changed, nil
	}
	if _, both := panel["queries"]; both {
		return false, fmt.Errorf(`spec: upgrade: %s has both "query" and "queries"`, path)
	}
	q := map[string]any{"query": query}
	if hasLegend {
		q["legend"] = legend
	}
	panel["queries"] = []any{q}
	delete(panel, "query")
	delete(panel, "legend")
	return true, nil
}

func flattenDatasource(owner map[string]any, path string) (changed bool, err error) {
	ref, ok := owner["datasource"].(map[string]any)
	if !ok {
		return false, nil
	}
	uid, ok := ref["uid"].(string)
	if !ok {
		return false, fmt.Errorf("spec: upgrade: %s/datasource has no string uid", path)
	}
	owner["datasource"] = uid
	return true, nil
}
