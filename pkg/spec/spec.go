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
	SpecVersion          string     `json:"specVersion"`
	Title                string     `json:"title"`
	Description          string     `json:"description,omitempty"`
	Tags                 []string   `json:"tags,omitempty"`
	Time                 TimeRange  `json:"time"`
	Refresh              string     `json:"refresh,omitempty"`
	Timezone             string     `json:"timezone,omitempty"`
	WeekStart            WeekStart  `json:"weekStart,omitempty"`
	FiscalYearStartMonth int        `json:"fiscalYearStartMonth,omitempty"`
	NowDelay             string     `json:"nowDelay,omitempty"`
	CursorSync           CursorSync `json:"cursorSync,omitempty"`
	Variables            []Variable `json:"variables,omitempty"`
	Links                []Link     `json:"links,omitempty"`
	Panels               []Panel    `json:"panels"`
}

type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Variable struct {
	Name             string         `json:"name"`
	Label            string         `json:"label,omitempty"`
	Description      string         `json:"description,omitempty"`
	Hide             string         `json:"hide,omitempty"`
	Type             VariableType   `json:"type"`
	Datasource       string         `json:"datasource,omitempty"`
	DatasourceType   DatasourceType `json:"datasourceType,omitempty"`
	Query            string         `json:"query,omitempty"`
	Regex            string         `json:"regex,omitempty"`
	Sort             string         `json:"sort,omitempty"`
	Refresh          string         `json:"refresh,omitempty"`
	Options          []string       `json:"options,omitempty"`
	Multi            bool           `json:"multi,omitempty"`
	IncludeAll       bool           `json:"includeAll,omitempty"`
	AllValue         string         `json:"allValue,omitempty"`
	AllowCustomValue bool           `json:"allowCustomValue,omitempty"`
	Value            string         `json:"value,omitempty"`
	Auto             bool           `json:"auto,omitempty"`
	AutoSteps        int            `json:"autoSteps,omitempty"`
	AutoMin          string         `json:"autoMin,omitempty"`
	Default          *Selection     `json:"default,omitempty"`
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
	Ref            string `json:"ref,omitempty"`
	Query          string `json:"query"`
	Legend         string `json:"legend,omitempty"`
	Datasource     string `json:"datasource,omitempty"`
	Instant        bool   `json:"instant,omitempty"`
	Format         string `json:"format,omitempty"`
	Interval       string `json:"interval,omitempty"`
	IntervalFactor int    `json:"intervalFactor,omitempty"`
	Exemplar       bool   `json:"exemplar,omitempty"`
	Hide           bool   `json:"hide,omitempty"`
}

type FieldProps struct {
	Unit              string      `json:"unit,omitempty"`
	Decimals          *int        `json:"decimals,omitempty"`
	Min               *float64    `json:"min,omitempty"`
	Max               *float64    `json:"max,omitempty"`
	Thresholds        []Threshold `json:"thresholds,omitempty"`
	ThresholdMode     string      `json:"thresholdMode,omitempty"`
	Color             string      `json:"color,omitempty"`
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
	PieType            string             `json:"pieType,omitempty"`
	Calculate          *bool              `json:"calculate,omitempty"`
	YScale             string             `json:"yScale,omitempty"`
	BucketSize         *float64           `json:"bucketSize,omitempty"`
	BucketCount        int                `json:"bucketCount,omitempty"`
	MergeValues        *bool              `json:"mergeValues,omitempty"`
	ShowValues         string             `json:"showValues,omitempty"`
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
