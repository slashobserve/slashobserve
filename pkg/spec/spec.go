package spec

import (
	"bytes"
	"encoding/json"
	"slices"
	"sync"

	"github.com/slashobserve/slashobserve/spec/schema"
)

const (
	Version     = "0"
	GridColumns = 24
	MaxBytes    = 1 << 20
)

type DatasourceType string

const (
	Prometheus DatasourceType = "prometheus"
	ClickHouse DatasourceType = "clickhouse"
)

type PanelType string

const (
	Timeseries PanelType = "timeseries"
	Stat       PanelType = "stat"
	Table      PanelType = "table"
	BarGauge   PanelType = "bargauge"
)

type VariableType string

const (
	QueryVariable  VariableType = "query"
	CustomVariable VariableType = "custom"
)

type Dashboard struct {
	SpecVersion string     `json:"specVersion"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	TimeRange   TimeRange  `json:"timeRange"`
	Refresh     string     `json:"refresh,omitempty"`
	Variables   []Variable `json:"variables,omitempty"`
	Panels      []Panel    `json:"panels"`
}

type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Variable struct {
	Name       string         `json:"name"`
	Label      string         `json:"label,omitempty"`
	Type       VariableType   `json:"type"`
	Datasource *DatasourceRef `json:"datasource,omitempty"`
	Query      string         `json:"query,omitempty"`
	Options    []string       `json:"options,omitempty"`
	Multi      bool           `json:"multi,omitempty"`
	IncludeAll bool           `json:"includeAll,omitempty"`
}

type Panel struct {
	ID          string        `json:"id"`
	Type        PanelType     `json:"type"`
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	GridPos     GridPos       `json:"gridPos"`
	Datasource  DatasourceRef `json:"datasource"`
	Query       string        `json:"query"`
	Legend      string        `json:"legend,omitempty"`
	Unit        string        `json:"unit,omitempty"`
	Decimals    *int          `json:"decimals,omitempty"`
	Min         *float64      `json:"min,omitempty"`
	Max         *float64      `json:"max,omitempty"`
	Thresholds  []Threshold   `json:"thresholds,omitempty"`
}

type GridPos struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type DatasourceRef struct {
	Type DatasourceType `json:"type"`
	UID  string         `json:"uid"`
}

type Threshold struct {
	Value float64 `json:"value"`
	Color string  `json:"color"`
}

func Schema() []byte {
	return bytes.Clone(schema.ObserveSpecV0)
}

var units = sync.OnceValue(func() []string {
	var doc struct {
		Defs struct {
			Unit struct {
				Enum []string `json:"enum"`
			} `json:"unit"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.ObserveSpecV0, &doc); err != nil {
		panic("spec: embedded schema is not valid JSON: " + err.Error())
	}
	return doc.Defs.Unit.Enum
})

func Units() []string {
	return slices.Clone(units())
}
