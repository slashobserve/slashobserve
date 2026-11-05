package cli

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

type exploreRequest struct {
	Query string `json:"query"`
	Kind  string `json:"kind"`
	From  string `json:"from"`
	To    string `json:"to"`
	Limit int    `json:"limit,omitempty"`
}

type frameField struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Values []json.RawMessage `json:"values"`
}

type frame struct {
	Ref    string            `json:"ref"`
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
	Fields []frameField      `json:"fields"`
}

type exploreResponse struct {
	Frames []frame `json:"frames"`
}

func explore(ctx context.Context, c *client, uid string, req exploreRequest) ([]byte, exploreResponse, error) {
	data, err := c.post(ctx, "datasources/"+url.PathEscape(uid)+"/query", req)
	if err != nil {
		return nil, exploreResponse{}, err
	}
	body, err := decode[exploreResponse](data)
	if err != nil {
		return nil, exploreResponse{}, err
	}
	return data, body, nil
}

func fieldIndex(f frame, names ...string) int {
	for _, name := range names {
		for i, field := range f.Fields {
			if strings.EqualFold(field.Name, name) {
				return i
			}
		}
	}
	return -1
}

func firstOfType(f frame, typ string) int {
	for i, field := range f.Fields {
		if field.Type == typ {
			return i
		}
	}
	return -1
}

func datasourceType(ctx context.Context, c *client, uid string) (string, error) {
	list, err := listDatasources(ctx, c)
	if err != nil {
		return "", err
	}
	for _, ds := range list {
		if ds.UID == uid {
			return ds.Type, nil
		}
	}
	return "", usagef("no datasource has uid %q; observe doctor lists them", text(uid))
}

func (f frame) rows() int {
	n := 0
	for _, field := range f.Fields {
		n = max(n, len(field.Values))
	}
	return n
}

func (f frame) raw(field, row int) json.RawMessage {
	if field < 0 || field >= len(f.Fields) || row >= len(f.Fields[field].Values) {
		return nil
	}
	return f.Fields[field].Values[row]
}

func (f frame) stringAt(field, row int) string {
	raw := f.raw(field, row)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func (f frame) numberAt(field, row int) (float64, bool) {
	raw := f.raw(field, row)
	var n float64
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	return n, true
}
