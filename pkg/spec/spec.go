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

type WeekStart string

const (
	WeekStartMonday   WeekStart = "monday"
	WeekStartSunday   WeekStart = "sunday"
	WeekStartSaturday WeekStart = "saturday"
)

const (
	TimezoneBrowser = "browser"
	TimezoneUTC     = "utc"
)

type CursorSync string

const (
	CursorSyncOff       CursorSync = "off"
	CursorSyncCrosshair CursorSync = "crosshair"
	CursorSyncTooltip   CursorSync = "tooltip"
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
	News          PanelType = "news"
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
	return t.IsData() || t == Text || t == DashList || t == News
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
	SpecVersion          string          `json:"specVersion"`
	Title                string          `json:"title"`
	Description          string          `json:"description,omitempty"`
	Tags                 []string        `json:"tags,omitempty"`
	Time                 TimeRange       `json:"time"`
	Refresh              string          `json:"refresh,omitempty"`
	TimePicker           *TimePicker     `json:"timePicker,omitempty"`
	LiveNow              bool            `json:"liveNow,omitempty"`
	Timezone             string          `json:"timezone,omitempty"`
	WeekStart            WeekStart       `json:"weekStart,omitempty"`
	FiscalYearStartMonth int             `json:"fiscalYearStartMonth,omitempty"`
	NowDelay             string          `json:"nowDelay,omitempty"`
	CursorSync           CursorSync      `json:"cursorSync,omitempty"`
	Variables            []Variable      `json:"variables,omitempty"`
	Links                []DashboardLink `json:"links,omitempty"`
	Panels               []Panel         `json:"panels"`
}

type TimePicker struct {
	Hidden           bool         `json:"hidden,omitempty"`
	RefreshIntervals []string     `json:"refreshIntervals,omitempty"`
	QuickRanges      []QuickRange `json:"quickRanges,omitempty"`
}

type QuickRange struct {
	Display string `json:"display"`
	From    string `json:"from"`
	To      string `json:"to"`
}

const DashboardsLinkType = "dashboards"

type DashboardLink struct {
	Type        string   `json:"type,omitempty"`
	Title       string   `json:"title"`
	URL         string   `json:"url,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	AsDropdown  bool     `json:"asDropdown,omitempty"`
	NewTab      bool     `json:"newTab,omitempty"`
	KeepTime    bool     `json:"keepTime,omitempty"`
	IncludeVars bool     `json:"includeVars,omitempty"`
}

type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Variable struct {
	Name             string           `json:"name"`
	Label            string           `json:"label,omitempty"`
	Description      string           `json:"description,omitempty"`
	Hide             string           `json:"hide,omitempty"`
	Type             VariableType     `json:"type"`
	Datasource       string           `json:"datasource,omitempty"`
	DatasourceType   DatasourceType   `json:"datasourceType,omitempty"`
	Query            string           `json:"query,omitempty"`
	Regex            string           `json:"regex,omitempty"`
	Sort             string           `json:"sort,omitempty"`
	Refresh          string           `json:"refresh,omitempty"`
	Options          []VariableOption `json:"options,omitempty"`
	Multi            bool             `json:"multi,omitempty"`
	IncludeAll       bool             `json:"includeAll,omitempty"`
	AllValue         string           `json:"allValue,omitempty"`
	AllowCustomValue bool             `json:"allowCustomValue,omitempty"`
	ApplyToAll       bool             `json:"applyToAll,omitempty"`
	Value            string           `json:"value,omitempty"`
	Auto             bool             `json:"auto,omitempty"`
	AutoSteps        int              `json:"autoSteps,omitempty"`
	AutoMin          string           `json:"autoMin,omitempty"`
	Default          *Selection       `json:"default,omitempty"`
}

type VariableOption struct {
	Text  string
	Value string
}

func PlainOption(value string) VariableOption { return VariableOption{Text: value, Value: value} }

func PlainOptions(values ...string) []VariableOption {
	out := make([]VariableOption, len(values))
	for i, value := range values {
		out[i] = PlainOption(value)
	}
	return out
}

func OptionValues(options []VariableOption) []string {
	out := make([]string, len(options))
	for i, option := range options {
		out[i] = option.Value
	}
	return out
}

func OptionTexts(options []VariableOption) []string {
	out := make([]string, len(options))
	for i, option := range options {
		out[i] = option.Text
	}
	return out
}

func (o VariableOption) MarshalJSON() ([]byte, error) {
	if o.Text == o.Value {
		return json.Marshal(o.Value)
	}
	return json.Marshal(struct {
		Text  string `json:"text"`
		Value string `json:"value"`
	}{o.Text, o.Value})
}

func (o *VariableOption) UnmarshalJSON(b []byte) error {
	var plain string
	if err := json.Unmarshal(b, &plain); err == nil {
		*o = PlainOption(plain)
		return nil
	}
	var pair struct {
		Text  string `json:"text"`
		Value string `json:"value"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pair); err != nil {
		return errors.New("spec: an option must be a string or an object with text and value")
	}
	*o = VariableOption(pair)
	return nil
}

type Selection struct {
	Values  []string
	List    bool
	Filters []AdhocFilter
}

type AdhocFilter struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

func (s Selection) MarshalJSON() ([]byte, error) {
	if len(s.Filters) > 0 {
		return json.Marshal(s.Filters)
	}
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
	if err := json.Unmarshal(b, &many); err == nil {
		*s = Selection{Values: many, List: true}
		return nil
	}
	var filters []AdhocFilter
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&filters); err != nil {
		return errors.New("spec: selection must be a string, an array of strings or an array of filters")
	}
	*s = Selection{Filters: filters}
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
	Transparent     bool        `json:"transparent,omitempty"`
	TitleHidden     bool        `json:"titleHidden,omitempty"`
	Display         *Display    `json:"display,omitempty"`
	Content         *string     `json:"content,omitempty"`
	Mode            string      `json:"mode,omitempty"`
	Code            *TextCode   `json:"code,omitempty"`
	Reason          string      `json:"reason,omitempty"`
	SourceType      string      `json:"sourceType,omitempty"`
	Collapsed       bool        `json:"collapsed,omitempty"`
	Panels          []Panel     `json:"panels,omitempty"`
}

type Query struct {
	Ref            string      `json:"ref,omitempty"`
	Query          string      `json:"query,omitempty"`
	Legend         string      `json:"legend,omitempty"`
	Datasource     string      `json:"datasource,omitempty"`
	Instant        bool        `json:"instant,omitempty"`
	Format         string      `json:"format,omitempty"`
	Interval       string      `json:"interval,omitempty"`
	IntervalFactor int         `json:"intervalFactor,omitempty"`
	Exemplar       bool        `json:"exemplar,omitempty"`
	Hide           bool        `json:"hide,omitempty"`
	Expression     *Expression `json:"expression,omitempty"`
}

type Expression struct {
	Type        string    `json:"type"`
	Math        string    `json:"math,omitempty"`
	Input       string    `json:"input,omitempty"`
	Reducer     string    `json:"reducer,omitempty"`
	Mode        string    `json:"mode,omitempty"`
	ReplaceWith *float64  `json:"replaceWith,omitempty"`
	Window      string    `json:"window,omitempty"`
	Downsampler string    `json:"downsampler,omitempty"`
	Upsampler   string    `json:"upsampler,omitempty"`
	Evaluator   string    `json:"evaluator,omitempty"`
	Params      []float64 `json:"params,omitempty"`
}

type FieldProps struct {
	Unit              string      `json:"unit,omitempty"`
	Decimals          *int        `json:"decimals,omitempty"`
	Min               *float64    `json:"min,omitempty"`
	Max               *float64    `json:"max,omitempty"`
	FieldMinMax       *bool       `json:"fieldMinMax,omitempty"`
	Thresholds        []Threshold `json:"thresholds,omitempty"`
	ThresholdMode     string      `json:"thresholdMode,omitempty"`
	Color             string      `json:"color,omitempty"`
	ColorMode         string      `json:"colorMode,omitempty"`
	ColorScheme       string      `json:"colorScheme,omitempty"`
	ColorBy           string      `json:"colorBy,omitempty"`
	DisplayName       string      `json:"displayName,omitempty"`
	NoValue           string      `json:"noValue,omitempty"`
	Mappings          []Mapping   `json:"mappings,omitempty"`
	Links             []Link      `json:"links,omitempty"`
	Style             string      `json:"style,omitempty"`
	Fill              *int        `json:"fill,omitempty"`
	LineStyle         string      `json:"lineStyle,omitempty"`
	LineInterpolation string      `json:"lineInterpolation,omitempty"`
	LineWidth         *int        `json:"lineWidth,omitempty"`
	Points            string      `json:"points,omitempty"`
	PointSize         *int        `json:"pointSize,omitempty"`
	PointShape        string      `json:"pointShape,omitempty"`
	PointStrokeWidth  *int        `json:"pointStrokeWidth,omitempty"`
	SpanNulls         any         `json:"spanNulls,omitempty"`
	Stack             string      `json:"stack,omitempty"`
	Axis              string      `json:"axis,omitempty"`
	Hidden            *bool       `json:"hidden,omitempty"`
	AxisLabel         string      `json:"axisLabel,omitempty"`
	AxisSoftMin       *float64    `json:"axisSoftMin,omitempty"`
	AxisSoftMax       *float64    `json:"axisSoftMax,omitempty"`
	Scale             string      `json:"scale,omitempty"`
	LogBase           *int        `json:"logBase,omitempty"`
	LinearThreshold   *float64    `json:"linearThreshold,omitempty"`
	AxisCenteredZero  *bool       `json:"axisCenteredZero,omitempty"`
	AxisWidth         *int        `json:"axisWidth,omitempty"`
	AxisGrid          *bool       `json:"axisGrid,omitempty"`
	AxisBorder        *bool       `json:"axisBorder,omitempty"`
	AxisColor         string      `json:"axisColor,omitempty"`
	Cell              *Cell       `json:"cell,omitempty"`
	Width             int         `json:"width,omitempty"`
	MinWidth          int         `json:"minWidth,omitempty"`
	Align             string      `json:"align,omitempty"`
	Filterable        *bool       `json:"filterable,omitempty"`
	Inspect           *bool       `json:"inspect,omitempty"`
	HideHeader        *bool       `json:"hideHeader,omitempty"`
	ValueTransform    string      `json:"valueTransform,omitempty"`
	HideInLegend      *bool       `json:"hideInLegend,omitempty"`
	HideInTooltip     *bool       `json:"hideInTooltip,omitempty"`
	StackGroup        string      `json:"stackGroup,omitempty"`
	FillBelowTo       string      `json:"fillBelowTo,omitempty"`
	InsertNulls       int64       `json:"insertNulls,omitempty"`
	NullAsZero        *bool       `json:"nullAsZero,omitempty"`
	BarAlignment      string      `json:"barAlignment,omitempty"`
	BarWidthFactor    *float64    `json:"barWidthFactor,omitempty"`
	BarMaxWidth       *int        `json:"barMaxWidth,omitempty"`
	ThresholdsStyle   string      `json:"thresholdsStyle,omitempty"`
}

type Cell struct {
	Type       string `json:"type"`
	GaugeStyle string `json:"gaugeStyle,omitempty"`
	ValueMode  string `json:"valueMode,omitempty"`
	WholeRow   *bool  `json:"wholeRow,omitempty"`
	Wrap       *bool  `json:"wrap,omitempty"`
	ImageAlt   string `json:"imageAlt,omitempty"`
	ImageTitle string `json:"imageTitle,omitempty"`
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
	Title    string `json:"title"`
	URL      string `json:"url"`
	NewTab   bool   `json:"newTab,omitempty"`
	OneClick bool   `json:"oneClick,omitempty"`
}

type Override struct {
	Match Match `json:"match"`
	FieldProps
}

type Match struct {
	Query       string      `json:"query,omitempty"`
	Series      string      `json:"series,omitempty"`
	SeriesRegex string      `json:"seriesRegex,omitempty"`
	Field       string      `json:"field,omitempty"`
	FieldType   string      `json:"fieldType,omitempty"`
	Value       *ValueMatch `json:"value,omitempty"`
}

type ValueMatch struct {
	Reducer string  `json:"reducer"`
	Op      string  `json:"op"`
	Value   float64 `json:"value"`
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
	Conversions  []Conversion      `json:"conversions,omitempty"`
	Format       string            `json:"format,omitempty"`
	Start        *int              `json:"start,omitempty"`
	End          *int              `json:"end,omitempty"`
	UTC          bool              `json:"utc,omitempty"`
	Regex        string            `json:"regex,omitempty"`
	RenameTo     string            `json:"renameTo,omitempty"`
	Keep         string            `json:"keep,omitempty"`
	Match        string            `json:"match,omitempty"`
	Filters      []ValueFilter     `json:"filters,omitempty"`
	Replace      bool              `json:"replace,omitempty"`
	KeepTime     bool              `json:"keepTime,omitempty"`
	Paths        []FieldPath       `json:"paths,omitempty"`
	Delimiter    string            `json:"delimiter,omitempty"`
	ValueLabel   string            `json:"valueLabel,omitempty"`
	Value        string            `json:"value,omitempty"`
	On           []string          `json:"on,omitempty"`
	Fields       []string          `json:"fields,omitempty"`
	KeepFields   bool              `json:"keepFields,omitempty"`
	Naming       string            `json:"naming,omitempty"`
	Stats        map[string]string `json:"stats,omitempty"`
	Mappings     []ConfigMapping   `json:"mappings,omitempty"`
	ConfigQuery  string            `json:"configQuery,omitempty"`
	ApplyTo      *Match            `json:"applyTo,omitempty"`
	NameMode     string            `json:"nameMode,omitempty"`
	NameLabel    string            `json:"nameLabel,omitempty"`
	By           []string          `json:"by,omitempty"`
	Aggregate    []Aggregation     `json:"aggregate,omitempty"`
	HideHeaders  bool              `json:"hideHeaders,omitempty"`
	Column       string            `json:"column,omitempty"`
	Row          string            `json:"row,omitempty"`
	Empty        string            `json:"empty,omitempty"`
	BucketCount  int               `json:"bucketCount,omitempty"`
	BucketSize   float64           `json:"bucketSize,omitempty"`
	BucketOffset float64           `json:"bucketOffset,omitempty"`
	Combine      bool              `json:"combine,omitempty"`
	Model        string            `json:"model,omitempty"`
	Degree       int               `json:"degree,omitempty"`
	X            string            `json:"x,omitempty"`
	Y            string            `json:"y,omitempty"`
	Predictions  int               `json:"predictions,omitempty"`
	FirstName    string            `json:"firstName,omitempty"`
	RestName     string            `json:"restName,omitempty"`
}

type Aggregation struct {
	Field    string   `json:"field"`
	Reducers []string `json:"reducers"`
}

type ConfigMapping struct {
	Field   string `json:"field"`
	To      string `json:"to"`
	Reducer string `json:"reducer,omitempty"`
}

type Conversion struct {
	Field      string `json:"field"`
	To         string `json:"to"`
	DateFormat string `json:"dateFormat,omitempty"`
	JoinWith   string `json:"joinWith,omitempty"`
}

type ValueFilter struct {
	Field string   `json:"field"`
	Op    string   `json:"op"`
	Value any      `json:"value,omitempty"`
	From  *float64 `json:"from,omitempty"`
	To    *float64 `json:"to,omitempty"`
}

type FieldPath struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

type Buckets struct {
	Mode  string   `json:"mode"`
	Value *float64 `json:"value,omitempty"`
}

type Display struct {
	Legend             string             `json:"legend,omitempty"`
	LegendValues       []string           `json:"legendValues,omitempty"`
	LegendMode         string             `json:"legendMode,omitempty"`
	LegendSortBy       string             `json:"legendSortBy,omitempty"`
	LegendSortDesc     *bool              `json:"legendSortDesc,omitempty"`
	LegendWidth        int                `json:"legendWidth,omitempty"`
	LegendHideZero     *bool              `json:"legendHideZero,omitempty"`
	LegendHideEmpty    *bool              `json:"legendHideEmpty,omitempty"`
	Tooltip            string             `json:"tooltip,omitempty"`
	TooltipSort        string             `json:"tooltipSort,omitempty"`
	TooltipHideZeros   *bool              `json:"tooltipHideZeros,omitempty"`
	TooltipCumulative  *bool              `json:"tooltipCumulative,omitempty"`
	TooltipMaxWidth    int                `json:"tooltipMaxWidth,omitempty"`
	TooltipMaxHeight   int                `json:"tooltipMaxHeight,omitempty"`
	XAxis              string             `json:"xAxis,omitempty"`
	IncludeAllFields   *bool              `json:"includeAllFields,omitempty"`
	Reduce             string             `json:"reduce,omitempty"`
	AllValues          *bool              `json:"allValues,omitempty"`
	Fields             string             `json:"fields,omitempty"`
	ColorMode          string             `json:"colorMode,omitempty"`
	Graph              *bool              `json:"graph,omitempty"`
	TextMode           string             `json:"textMode,omitempty"`
	Orientation        string             `json:"orientation,omitempty"`
	Justify            string             `json:"justify,omitempty"`
	TitleSize          int                `json:"titleSize,omitempty"`
	ValueSize          int                `json:"valueSize,omitempty"`
	PercentChange      *bool              `json:"percentChange,omitempty"`
	PercentChangeColor string             `json:"percentChangeColor,omitempty"`
	WideLayout         *bool              `json:"wideLayout,omitempty"`
	ThresholdMarkers   *bool              `json:"thresholdMarkers,omitempty"`
	ThresholdLabels    *bool              `json:"thresholdLabels,omitempty"`
	BarStyle           string             `json:"barStyle,omitempty"`
	ValueMode          string             `json:"valueMode,omitempty"`
	NamePlacement      string             `json:"namePlacement,omitempty"`
	ShowUnfilled       *bool              `json:"showUnfilled,omitempty"`
	MinWidth           int                `json:"minWidth,omitempty"`
	MinHeight          int                `json:"minHeight,omitempty"`
	MaxHeight          int                `json:"maxHeight,omitempty"`
	Header             *bool              `json:"header,omitempty"`
	Footer             []string           `json:"footer,omitempty"`
	FooterFields       []string           `json:"footerFields,omitempty"`
	FooterCountRows    *bool              `json:"footerCountRows,omitempty"`
	CellHeight         string             `json:"cellHeight,omitempty"`
	Pagination         *bool              `json:"pagination,omitempty"`
	FrozenColumns      int                `json:"frozenColumns,omitempty"`
	SortBy             string             `json:"sortBy,omitempty"`
	SortDesc           *bool              `json:"sortDesc,omitempty"`
	TypeIcons          *bool              `json:"typeIcons,omitempty"`
	PieType            string             `json:"pieType,omitempty"`
	SliceLabels        []string           `json:"sliceLabels,omitempty"`
	LegendShow         *[]string          `json:"legendShow,omitempty"`
	Calculate          *bool              `json:"calculate,omitempty"`
	YScale             string             `json:"yScale,omitempty"`
	ColorScheme        string             `json:"colorScheme,omitempty"`
	ColorSteps         int                `json:"colorSteps,omitempty"`
	ColorScale         string             `json:"colorScale,omitempty"`
	ColorExponent      *float64           `json:"colorExponent,omitempty"`
	ColorFill          string             `json:"colorFill,omitempty"`
	ColorMin           *float64           `json:"colorMin,omitempty"`
	ColorMax           *float64           `json:"colorMax,omitempty"`
	ColorReverse       *bool              `json:"colorReverse,omitempty"`
	CellGap            int                `json:"cellGap,omitempty"`
	CellRadius         int                `json:"cellRadius,omitempty"`
	CellUnit           string             `json:"cellUnit,omitempty"`
	CellDecimals       *int               `json:"cellDecimals,omitempty"`
	HideBelow          *float64           `json:"hideBelow,omitempty"`
	HideAbove          *float64           `json:"hideAbove,omitempty"`
	XBuckets           *Buckets           `json:"xBuckets,omitempty"`
	YBuckets           *Buckets           `json:"yBuckets,omitempty"`
	YBucketScale       string             `json:"yBucketScale,omitempty"`
	YBucketLogBase     int                `json:"yBucketLogBase,omitempty"`
	RowsLayout         string             `json:"rowsLayout,omitempty"`
	YUnit              string             `json:"yUnit,omitempty"`
	YDecimals          *int               `json:"yDecimals,omitempty"`
	YMin               *float64           `json:"yMin,omitempty"`
	YMax               *float64           `json:"yMax,omitempty"`
	YReverse           *bool              `json:"yReverse,omitempty"`
	YAxis              string             `json:"yAxis,omitempty"`
	YAxisWidth         *int               `json:"yAxisWidth,omitempty"`
	YAxisLabel         string             `json:"yAxisLabel,omitempty"`
	TooltipHistogram   *bool              `json:"tooltipHistogram,omitempty"`
	TooltipColorScale  *bool              `json:"tooltipColorScale,omitempty"`
	Exemplars          *bool              `json:"exemplars,omitempty"`
	ExemplarColor      string             `json:"exemplarColor,omitempty"`
	BucketSize         *float64           `json:"bucketSize,omitempty"`
	BucketCount        int                `json:"bucketCount,omitempty"`
	BucketOffset       *float64           `json:"bucketOffset,omitempty"`
	Combine            *bool              `json:"combine,omitempty"`
	Stack              string             `json:"stack,omitempty"`
	XTickRotation      int                `json:"xTickRotation,omitempty"`
	XTickSpacing       int                `json:"xTickSpacing,omitempty"`
	XTickMaxLength     int                `json:"xTickMaxLength,omitempty"`
	BarRadius          float64            `json:"barRadius,omitempty"`
	FullHighlight      *bool              `json:"fullHighlight,omitempty"`
	ColorByField       string             `json:"colorByField,omitempty"`
	MergeValues        *bool              `json:"mergeValues,omitempty"`
	ShowValues         string             `json:"showValues,omitempty"`
	RowHeight          *float64           `json:"rowHeight,omitempty"`
	ColumnWidth        *float64           `json:"columnWidth,omitempty"`
	AlignValue         string             `json:"alignValue,omitempty"`
	Wrap               *bool              `json:"wrap,omitempty"`
	ShowTime           *bool              `json:"showTime,omitempty"`
	Order              string             `json:"order,omitempty"`
	ShowLabels         *bool              `json:"showLabels,omitempty"`
	ShowCommonLabels   *bool              `json:"showCommonLabels,omitempty"`
	Details            *bool              `json:"details,omitempty"`
	Prettify           *bool              `json:"prettify,omitempty"`
	Dedup              string             `json:"dedup,omitempty"`
	Controls           *bool              `json:"controls,omitempty"`
	FontSize           string             `json:"fontSize,omitempty"`
	Highlight          *bool              `json:"highlight,omitempty"`
	DetailsMode        string             `json:"detailsMode,omitempty"`
	XField             string             `json:"xField,omitempty"`
	BarWidth           *float64           `json:"barWidth,omitempty"`
	GroupWidth         *float64           `json:"groupWidth,omitempty"`
	Show               string             `json:"show,omitempty"`
	PointSize          int                `json:"pointSize,omitempty"`
	PointSizeMin       int                `json:"pointSizeMin,omitempty"`
	PointSizeMax       int                `json:"pointSizeMax,omitempty"`
	Series             []XYSeries         `json:"series,omitempty"`
	Mode               string             `json:"mode,omitempty"`
	CandleStyle        string             `json:"candleStyle,omitempty"`
	ColorStrategy      string             `json:"colorStrategy,omitempty"`
	UpColor            string             `json:"upColor,omitempty"`
	DownColor          string             `json:"downColor,omitempty"`
	Open               string             `json:"open,omitempty"`
	High               string             `json:"high,omitempty"`
	Low                string             `json:"low,omitempty"`
	Close              string             `json:"close,omitempty"`
	Volume             string             `json:"volume,omitempty"`
	Layout             string             `json:"layout,omitempty"`
	WheelZoom          *bool              `json:"wheelZoom,omitempty"`
	View               string             `json:"view,omitempty"`
	Lat                *float64           `json:"lat,omitempty"`
	Lon                *float64           `json:"lon,omitempty"`
	Zoom               *float64           `json:"zoom,omitempty"`
	Location           string             `json:"location,omitempty"`
	LatField           string             `json:"latField,omitempty"`
	LonField           string             `json:"lonField,omitempty"`
	GeohashField       string             `json:"geohashField,omitempty"`
	SizeField          string             `json:"sizeField,omitempty"`
	ColorField         string             `json:"colorField,omitempty"`
	MarkerSize         int                `json:"markerSize,omitempty"`
	MarkerSizeMax      int                `json:"markerSizeMax,omitempty"`
	Basemap            string             `json:"basemap,omitempty"`
	BasemapTheme       string             `json:"basemapTheme,omitempty"`
	BasemapLabels      *bool              `json:"basemapLabels,omitempty"`
	EsriServer         string             `json:"esriServer,omitempty"`
	BasemapURL         string             `json:"basemapUrl,omitempty"`
	BasemapAttribution string             `json:"basemapAttribution,omitempty"`
	BasemapOpacity     *float64           `json:"basemapOpacity,omitempty"`
	MinZoom            *float64           `json:"minZoom,omitempty"`
	MaxZoom            *float64           `json:"maxZoom,omitempty"`
	FitLayer           string             `json:"fitLayer,omitempty"`
	FitLastOnly        *bool              `json:"fitLastOnly,omitempty"`
	FitPadding         *int               `json:"fitPadding,omitempty"`
	SharedView         *bool              `json:"sharedView,omitempty"`
	ShowZoom           *bool              `json:"showZoom,omitempty"`
	MouseWheelZoom     *bool              `json:"mouseWheelZoom,omitempty"`
	ShowAttribution    *bool              `json:"showAttribution,omitempty"`
	ShowScale          *bool              `json:"showScale,omitempty"`
	ShowMeasure        *bool              `json:"showMeasure,omitempty"`
	ShowDebug          *bool              `json:"showDebug,omitempty"`
	Layers             []GeomapLayer      `json:"layers,omitempty"`
	BackgroundColor    string             `json:"backgroundColor,omitempty"`
	BackgroundImage    *ImageRef          `json:"backgroundImage,omitempty"`
	BackgroundSize     string             `json:"backgroundSize,omitempty"`
	BorderColor        string             `json:"borderColor,omitempty"`
	BorderWidth        *int               `json:"borderWidth,omitempty"`
	BorderRadius       *int               `json:"borderRadius,omitempty"`
	PanZoom            *bool              `json:"panZoom,omitempty"`
	InfinitePan        *bool              `json:"infinitePan,omitempty"`
	Elements           []CanvasElement    `json:"elements,omitempty"`
	Connections        []CanvasConnection `json:"connections,omitempty"`
	Limit              int                `json:"limit,omitempty"`
	Sort               string             `json:"sort,omitempty"`
	ViewMode           string             `json:"viewMode,omitempty"`
	Query              string             `json:"query,omitempty"`
	Tags               []string           `json:"tags,omitempty"`
	KeepTime           *bool              `json:"keepTime,omitempty"`
	IncludeVars        *bool              `json:"includeVars,omitempty"`
	FeedURL            string             `json:"feedUrl,omitempty"`
}

type XYSeries struct {
	Query string `json:"query,omitempty"`
	Frame string `json:"frame,omitempty"`
	X     string `json:"x,omitempty"`
	Y     string `json:"y,omitempty"`
	Size  string `json:"size,omitempty"`
	Color string `json:"color,omitempty"`
	Name  string `json:"name,omitempty"`
}

type CanvasElement struct {
	ID              string           `json:"id"`
	Kind            string           `json:"kind"`
	Left            float64          `json:"left"`
	Top             float64          `json:"top"`
	Width           float64          `json:"width"`
	Height          float64          `json:"height"`
	Text            string           `json:"text,omitempty"`
	Field           string           `json:"field,omitempty"`
	Fill            string           `json:"fill,omitempty"`
	Stroke          string           `json:"stroke,omitempty"`
	TextColor       string           `json:"textColor,omitempty"`
	FontSize        int              `json:"fontSize,omitempty"`
	Align           string           `json:"align,omitempty"`
	Placement       *CanvasPlacement `json:"placement,omitempty"`
	Rotation        *float64         `json:"rotation,omitempty"`
	VAlign          string           `json:"valign,omitempty"`
	BorderWidth     *int             `json:"borderWidth,omitempty"`
	Radius          *int             `json:"radius,omitempty"`
	BackgroundImage *ImageRef        `json:"backgroundImage,omitempty"`
	BackgroundSize  string           `json:"backgroundSize,omitempty"`
	Icon            string           `json:"icon,omitempty"`
	Image           *ImageRef        `json:"image,omitempty"`
	IconColor       string           `json:"iconColor,omitempty"`
	Server          string           `json:"server,omitempty"`
	ButtonStyle     string           `json:"buttonStyle,omitempty"`
	AccentColor     string           `json:"accentColor,omitempty"`
	RateField       string           `json:"rateField,omitempty"`
	YawField        string           `json:"yawField,omitempty"`
	PitchField      string           `json:"pitchField,omitempty"`
	RollField       string           `json:"rollField,omitempty"`
	Links           []Link           `json:"links,omitempty"`
	Actions         []Action         `json:"actions,omitempty"`
	OneClick        string           `json:"oneClick,omitempty"`
}

type CanvasPlacement struct {
	Horizontal string   `json:"horizontal"`
	Vertical   string   `json:"vertical"`
	Left       *float64 `json:"left,omitempty"`
	Right      *float64 `json:"right,omitempty"`
	Top        *float64 `json:"top,omitempty"`
	Bottom     *float64 `json:"bottom,omitempty"`
	Width      *float64 `json:"width,omitempty"`
	Height     *float64 `json:"height,omitempty"`
}

type CanvasConnection struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	Color      string         `json:"color,omitempty"`
	Field      string         `json:"field,omitempty"`
	Width      int            `json:"width,omitempty"`
	Path       string         `json:"path,omitempty"`
	Vertices   []CanvasVertex `json:"vertices,omitempty"`
	LineStyle  string         `json:"lineStyle,omitempty"`
	Arrow      string         `json:"arrow,omitempty"`
	WidthField string         `json:"widthField,omitempty"`
	MinWidth   int            `json:"minWidth,omitempty"`
	MaxWidth   int            `json:"maxWidth,omitempty"`
}

type CanvasVertex struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type TextCode struct {
	Language    string `json:"language,omitempty"`
	LineNumbers *bool  `json:"lineNumbers,omitempty"`
}

type ImageRef struct {
	URL   string `json:"url,omitempty"`
	Field string `json:"field,omitempty"`
}

type Action struct {
	Title       string        `json:"title"`
	Method      string        `json:"method,omitempty"`
	URL         string        `json:"url"`
	ContentType string        `json:"contentType,omitempty"`
	Body        string        `json:"body,omitempty"`
	Headers     []ActionParam `json:"headers,omitempty"`
	Query       []ActionParam `json:"query,omitempty"`
	Confirm     string        `json:"confirm,omitempty"`
}

type ActionParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type GeomapLayer struct {
	Type          string       `json:"type"`
	Name          string       `json:"name,omitempty"`
	Query         string       `json:"query,omitempty"`
	Opacity       *float64     `json:"opacity,omitempty"`
	Tooltip       *bool        `json:"tooltip,omitempty"`
	Location      string       `json:"location,omitempty"`
	LatField      string       `json:"latField,omitempty"`
	LonField      string       `json:"lonField,omitempty"`
	GeohashField  string       `json:"geohashField,omitempty"`
	LookupField   string       `json:"lookupField,omitempty"`
	Gazetteer     string       `json:"gazetteer,omitempty"`
	SizeField     string       `json:"sizeField,omitempty"`
	ColorField    string       `json:"colorField,omitempty"`
	MarkerSize    int          `json:"markerSize,omitempty"`
	MarkerSizeMax int          `json:"markerSizeMax,omitempty"`
	Color         string       `json:"color,omitempty"`
	Symbol        string       `json:"symbol,omitempty"`
	TextField     string       `json:"textField,omitempty"`
	TextSize      int          `json:"textSize,omitempty"`
	RotationField string       `json:"rotationField,omitempty"`
	WeightField   string       `json:"weightField,omitempty"`
	Radius        int          `json:"radius,omitempty"`
	Blur          *int         `json:"blur,omitempty"`
	URL           string       `json:"url,omitempty"`
	Source        string       `json:"source,omitempty"`
	Rules         []GeomapRule `json:"rules,omitempty"`
	ShowSun       *bool        `json:"showSun,omitempty"`
	NightColor    string       `json:"nightColor,omitempty"`
	Arrow         string       `json:"arrow,omitempty"`
	LineWidth     int          `json:"lineWidth,omitempty"`
	ImageField    string       `json:"imageField,omitempty"`
	Shape         string       `json:"shape,omitempty"`
	Border        *int         `json:"border,omitempty"`
	Theme         string       `json:"theme,omitempty"`
	Labels        *bool        `json:"labels,omitempty"`
	Server        string       `json:"server,omitempty"`
	Attribution   string       `json:"attribution,omitempty"`
}

type GeomapRule struct {
	Property  string    `json:"property"`
	Operation string    `json:"operation"`
	Value     RuleValue `json:"value"`
	Color     string    `json:"color,omitempty"`
	Opacity   *float64  `json:"opacity,omitempty"`
}

type RuleValue struct {
	Text     string
	Number   float64
	IsNumber bool
}

func (v RuleValue) MarshalJSON() ([]byte, error) {
	if v.IsNumber {
		return json.Marshal(v.Number)
	}
	return json.Marshal(v.Text)
}

func (v *RuleValue) UnmarshalJSON(b []byte) error {
	var text string
	if err := json.Unmarshal(b, &text); err == nil {
		*v = RuleValue{Text: text}
		return nil
	}
	var number float64
	if err := json.Unmarshal(b, &number); err != nil {
		return errors.New("spec: rule value must be a string or a number")
	}
	*v = RuleValue{Number: number, IsNumber: true}
	return nil
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
	var all []string
	for _, branch := range doc.Defs.Unit.AnyOf {
		all = append(all, branch.Enum...)
	}
	if len(all) == 0 {
		panic("spec: embedded schema has no unit enum")
	}
	return all
})

func Units() []string {
	return slices.Clone(units())
}
