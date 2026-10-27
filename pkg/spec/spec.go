package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"sync"

	"github.com/slashobserve/slashobserve/spec/schema"
)

const (
	Version     = "0"
	GridColumns = 24
	MaxBytes    = 1 << 20
	MaxPanels   = 200

	MaxSuggestedPanelBytes = 63 << 10
)

type DatasourceType string

const (
	Prometheus DatasourceType = "prometheus"
	ClickHouse DatasourceType = "clickhouse"
)

type PanelType string

const (
	Timeseries    PanelType = "timeseries"
	Stat          PanelType = "stat"
	Gauge         PanelType = "gauge"
	BarGauge      PanelType = "bargauge"
	Table         PanelType = "table"
	PieChart      PanelType = "piechart"
	Heatmap       PanelType = "heatmap"
	Histogram     PanelType = "histogram"
	StateTimeline PanelType = "state-timeline"
	Logs          PanelType = "logs"
	BarChart      PanelType = "barchart"
	StatusHistory PanelType = "status-history"
	Trend         PanelType = "trend"
	XYChart       PanelType = "xychart"
	Candlestick   PanelType = "candlestick"
	NodeGraph     PanelType = "nodeGraph"
	Traces        PanelType = "traces"
	FlameGraph    PanelType = "flamegraph"
	Geomap        PanelType = "geomap"
	Canvas        PanelType = "canvas"
	AlertList     PanelType = "alertlist"
	DashList      PanelType = "dashlist"
	Text          PanelType = "text"
	Row           PanelType = "row"
	Placeholder   PanelType = "placeholder"
)

func (t PanelType) IsData() bool {
	switch t {
	case Timeseries, Stat, Gauge, BarGauge, Table, PieChart, Heatmap, Histogram, StateTimeline, Logs,
		BarChart, StatusHistory, Trend, XYChart, Candlestick, NodeGraph, Traces, FlameGraph, Geomap, Canvas, AlertList:
		return true
	}
	return false
}

func (t PanelType) IsEditable() bool {
	return t.IsData() || t == Text || t == DashList
}

type VariableType string

const (
	QueryVariable      VariableType = "query"
	CustomVariable     VariableType = "custom"
	ConstantVariable   VariableType = "constant"
	IntervalVariable   VariableType = "interval"
	TextboxVariable    VariableType = "textbox"
	DatasourceVariable VariableType = "datasource"
	AdhocVariable      VariableType = "adhoc"
)

type Dashboard struct {
	SpecVersion string     `json:"specVersion"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Time        TimeRange  `json:"time"`
	Refresh     string     `json:"refresh,omitempty"`
	Timezone    string     `json:"timezone,omitempty"`
	Variables   []Variable `json:"variables,omitempty"`
	Links       []Link     `json:"links,omitempty"`
	Panels      []Panel    `json:"panels"`
}

type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Variable struct {
	Name           string         `json:"name"`
	Label          string         `json:"label,omitempty"`
	Description    string         `json:"description,omitempty"`
	Hide           string         `json:"hide,omitempty"`
	Type           VariableType   `json:"type"`
	Datasource     string         `json:"datasource,omitempty"`
	DatasourceType DatasourceType `json:"datasourceType,omitempty"`
	Query          string         `json:"query,omitempty"`
	Regex          string         `json:"regex,omitempty"`
	Sort           string         `json:"sort,omitempty"`
	Refresh        string         `json:"refresh,omitempty"`
	Options        []string       `json:"options,omitempty"`
	Multi          bool           `json:"multi,omitempty"`
	IncludeAll     bool           `json:"includeAll,omitempty"`
	AllValue       string         `json:"allValue,omitempty"`
	Value          string         `json:"value,omitempty"`
	Auto           bool           `json:"auto,omitempty"`
	AutoSteps      int            `json:"autoSteps,omitempty"`
	AutoMin        string         `json:"autoMin,omitempty"`
	Default        *Selection     `json:"default,omitempty"`
}

type Selection struct {
	Values []string
	List   bool
}

func (s Selection) MarshalJSON() ([]byte, error) {
	if s.List {
		return json.Marshal(append([]string{}, s.Values...))
	}
	if len(s.Values) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(s.Values[0])
}

func (s *Selection) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = Selection{Values: []string{one}}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return errors.New("spec: selection must be a string or an array of strings")
	}
	*s = Selection{Values: many, List: true}
	return nil
}

type Panel struct {
	ID          string    `json:"id"`
	Type        PanelType `json:"type"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	GridPos     GridPos   `json:"gridPos"`
	Datasource  string    `json:"datasource,omitempty"`
	Queries     []Query   `json:"queries,omitempty"`
	FieldProps
	Overrides       []Override  `json:"overrides,omitempty"`
	Transform       []Transform `json:"transform,omitempty"`
	Repeat          string      `json:"repeat,omitempty"`
	RepeatDirection string      `json:"repeatDirection,omitempty"`
	MaxPerRow       int         `json:"maxPerRow,omitempty"`
	TimeFrom        string      `json:"timeFrom,omitempty"`
	TimeShift       string      `json:"timeShift,omitempty"`
	Interval        string      `json:"interval,omitempty"`
	MaxDataPoints   int         `json:"maxDataPoints,omitempty"`
	Display         *Display    `json:"display,omitempty"`
	Content         *string     `json:"content,omitempty"`
	Reason          string      `json:"reason,omitempty"`
	SourceType      string      `json:"sourceType,omitempty"`
	Collapsed       bool        `json:"collapsed,omitempty"`
	Panels          []Panel     `json:"panels,omitempty"`
}

type Query struct {
	Ref        string `json:"ref,omitempty"`
	Query      string `json:"query"`
	Legend     string `json:"legend,omitempty"`
	Datasource string `json:"datasource,omitempty"`
	Instant    bool   `json:"instant,omitempty"`
	Format     string `json:"format,omitempty"`
	Interval   string `json:"interval,omitempty"`
	Hide       bool   `json:"hide,omitempty"`
}

type FieldProps struct {
	Unit          string      `json:"unit,omitempty"`
	Decimals      *int        `json:"decimals,omitempty"`
	Min           *float64    `json:"min,omitempty"`
	Max           *float64    `json:"max,omitempty"`
	Thresholds    []Threshold `json:"thresholds,omitempty"`
	ThresholdMode string      `json:"thresholdMode,omitempty"`
	Color         string      `json:"color,omitempty"`
	DisplayName   string      `json:"displayName,omitempty"`
	NoValue       string      `json:"noValue,omitempty"`
	Mappings      []Mapping   `json:"mappings,omitempty"`
	Links         []Link      `json:"links,omitempty"`
	Style         string      `json:"style,omitempty"`
	Fill          *int        `json:"fill,omitempty"`
	LineStyle     string      `json:"lineStyle,omitempty"`
	Stack         string      `json:"stack,omitempty"`
	Axis          string      `json:"axis,omitempty"`
	Hidden        *bool       `json:"hidden,omitempty"`
}

type Threshold struct {
	Value *float64 `json:"value,omitempty"`
	Color string   `json:"color"`
}

type Mapping struct {
	Value   string       `json:"value,omitempty"`
	Range   *[2]*float64 `json:"range,omitempty"`
	Regex   string       `json:"regex,omitempty"`
	Special string       `json:"special,omitempty"`
	Text    string       `json:"text,omitempty"`
	Color   string       `json:"color,omitempty"`
}

type Link struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	NewTab bool   `json:"newTab,omitempty"`
}

type Override struct {
	Match Match `json:"match"`
	FieldProps
}

type Match struct {
	Query       string `json:"query,omitempty"`
	Series      string `json:"series,omitempty"`
	SeriesRegex string `json:"seriesRegex,omitempty"`
	Field       string `json:"field,omitempty"`
	FieldType   string `json:"fieldType,omitempty"`
}

type Transform struct {
	Type         string            `json:"type"`
	Exclude      []string          `json:"exclude,omitempty"`
	Rename       map[string]string `json:"rename,omitempty"`
	Order        []string          `json:"order,omitempty"`
	Reducers     []string          `json:"reducers,omitempty"`
	Mode         string            `json:"mode,omitempty"`
	Field        string            `json:"field,omitempty"`
	Include      []string          `json:"include,omitempty"`
	IncludeRegex string            `json:"includeRegex,omitempty"`
	Alias        string            `json:"alias,omitempty"`
	Expr         string            `json:"expr,omitempty"`
	ReduceRow    string            `json:"reduceRow,omitempty"`
	Desc         bool              `json:"desc,omitempty"`
	Count        int               `json:"count,omitempty"`
}

type Display struct {
	Legend           string             `json:"legend,omitempty"`
	LegendValues     []string           `json:"legendValues,omitempty"`
	Tooltip          string             `json:"tooltip,omitempty"`
	Reduce           string             `json:"reduce,omitempty"`
	ColorMode        string             `json:"colorMode,omitempty"`
	Graph            *bool              `json:"graph,omitempty"`
	TextMode         string             `json:"textMode,omitempty"`
	Orientation      string             `json:"orientation,omitempty"`
	ThresholdMarkers *bool              `json:"thresholdMarkers,omitempty"`
	Header           *bool              `json:"header,omitempty"`
	Footer           []string           `json:"footer,omitempty"`
	PieType          string             `json:"pieType,omitempty"`
	Calculate        *bool              `json:"calculate,omitempty"`
	YScale           string             `json:"yScale,omitempty"`
	BucketSize       *float64           `json:"bucketSize,omitempty"`
	BucketCount      int                `json:"bucketCount,omitempty"`
	MergeValues      *bool              `json:"mergeValues,omitempty"`
	ShowValues       string             `json:"showValues,omitempty"`
	Wrap             *bool              `json:"wrap,omitempty"`
	ShowTime         *bool              `json:"showTime,omitempty"`
	Order            string             `json:"order,omitempty"`
	XField           string             `json:"xField,omitempty"`
	BarWidth         *float64           `json:"barWidth,omitempty"`
	GroupWidth       *float64           `json:"groupWidth,omitempty"`
	Show             string             `json:"show,omitempty"`
	PointSize        int                `json:"pointSize,omitempty"`
	Mode             string             `json:"mode,omitempty"`
	CandleStyle      string             `json:"candleStyle,omitempty"`
	ColorStrategy    string             `json:"colorStrategy,omitempty"`
	UpColor          string             `json:"upColor,omitempty"`
	DownColor        string             `json:"downColor,omitempty"`
	Open             string             `json:"open,omitempty"`
	High             string             `json:"high,omitempty"`
	Low              string             `json:"low,omitempty"`
	Close            string             `json:"close,omitempty"`
	Volume           string             `json:"volume,omitempty"`
	Layout           string             `json:"layout,omitempty"`
	View             string             `json:"view,omitempty"`
	Lat              *float64           `json:"lat,omitempty"`
	Lon              *float64           `json:"lon,omitempty"`
	Zoom             *float64           `json:"zoom,omitempty"`
	Location         string             `json:"location,omitempty"`
	LatField         string             `json:"latField,omitempty"`
	LonField         string             `json:"lonField,omitempty"`
	GeohashField     string             `json:"geohashField,omitempty"`
	SizeField        string             `json:"sizeField,omitempty"`
	ColorField       string             `json:"colorField,omitempty"`
	MarkerSize       int                `json:"markerSize,omitempty"`
	MarkerSizeMax    int                `json:"markerSizeMax,omitempty"`
	Elements         []CanvasElement    `json:"elements,omitempty"`
	Connections      []CanvasConnection `json:"connections,omitempty"`
	Limit            int                `json:"limit,omitempty"`
	Sort             string             `json:"sort,omitempty"`
	ViewMode         string             `json:"viewMode,omitempty"`
	Query            string             `json:"query,omitempty"`
	Tags             []string           `json:"tags,omitempty"`
	KeepTime         *bool              `json:"keepTime,omitempty"`
	IncludeVars      *bool              `json:"includeVars,omitempty"`
}

type CanvasElement struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Left      float64 `json:"left"`
	Top       float64 `json:"top"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Text      string  `json:"text,omitempty"`
	Field     string  `json:"field,omitempty"`
	Fill      string  `json:"fill,omitempty"`
	Stroke    string  `json:"stroke,omitempty"`
	TextColor string  `json:"textColor,omitempty"`
	FontSize  int     `json:"fontSize,omitempty"`
	Align     string  `json:"align,omitempty"`
}

type CanvasConnection struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Color string `json:"color,omitempty"`
	Field string `json:"field,omitempty"`
	Width int    `json:"width,omitempty"`
}

type GridPos struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func Schema() []byte {
	return bytes.Clone(schema.ObserveSpecV0)
}

var units = sync.OnceValue(func() []string {
	var doc struct {
		Defs struct {
			Unit struct {
				AnyOf []struct {
					Enum []string `json:"enum"`
				} `json:"anyOf"`
			} `json:"unit"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.ObserveSpecV0, &doc); err != nil {
		panic("spec: embedded schema is not valid JSON: " + err.Error())
	}
	if len(doc.Defs.Unit.AnyOf) == 0 || len(doc.Defs.Unit.AnyOf[0].Enum) == 0 {
		panic("spec: embedded schema has no unit enum")
	}
	return doc.Defs.Unit.AnyOf[0].Enum
})

func Units() []string {
	return slices.Clone(units())
}
