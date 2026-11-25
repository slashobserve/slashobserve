package findings

import (
	"encoding/json"
	"fmt"
)

const (
	MaxUnverified = 20

	UnverifiedFigure    = "figure"
	UnverifiedComponent = "component"
	UnverifiedOnset     = "onset"
)

type Unverified struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func encodeUnverified(items []Unverified) ([]byte, error) {
	if len(items) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode unverified items: %w", err)
	}
	return raw, nil
}

func UnverifiedOf(raw []byte) []Unverified {
	var items []Unverified
	if len(raw) == 0 || json.Unmarshal(raw, &items) != nil {
		return []Unverified{}
	}
	return items
}
