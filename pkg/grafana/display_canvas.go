package grafana

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	canvasElementLimit   = 50
	canvasConnectionMax  = 100
	canvasIDLimit        = 64
	canvasColumnPixels   = 60
	canvasRowPixels      = 38
	canvasRowGapPixels   = 8
	canvasHeaderPixels   = 32
	canvasMinFontSize    = 8
	canvasMaxFontSize    = 72
	canvasMinConnWidth   = 1
	canvasMaxConnWidth   = 10
	canvasTextLimit      = 200
	canvasUnnamedElement = "e_"

	canvasTypeReason       = "Canvas element type %s is not supported; it was left out."
	canvasFrameReason      = "Grouped elements were flattened."
	canvasPixelReason      = "Positions inside a group were converted to percentages of the panel and may shift."
	canvasPlacementReason  = "A canvas element needs a position and a size; it was left out."
	canvasRotationReason   = "Only rotations from -360 to 360 degrees are supported."
	canvasTemplateReason   = "Text templates are not supported; the template is shown as plain text."
	canvasFieldReason      = "A color taken from a field other than the element's own is not supported."
	canvasBorderReason     = "Only border widths from 0 to 20 are supported."
	canvasRadiusReason     = "Only corner radii from 0 to 100 are supported."
	canvasImageReason      = "Only images at https addresses written out in full, or taken from a field, are supported."
	canvasIconReason       = "This icon cannot be loaded; only the bundled icons and https images are supported."
	canvasImageModeReason  = "Image mappings are not supported."
	canvasFontReason       = "Only font sizes from 8 to 72 are supported."
	canvasAlignReason      = "Only left, center and right alignment is supported."
	canvasElementsReason   = "A canvas holds at most 50 elements."
	canvasConnectionReason = "A connection to an element that was left out, or to itself, is not kept."
	canvasConnLimitReason  = "A canvas holds at most 100 connections."
	canvasConnSizeReason   = "Only connection widths from 1 to 10 are supported."
	canvasConnModeReason   = "Connection widths and directions that follow a rule are not supported."
	canvasVerticesReason   = "Bends in a connection are not supported yet."
	canvasMaxBorderWidth   = 20
	canvasMaxRadius        = 100
	canvasMaxRotation      = 360
	bundledIconPrefix      = "img/icons/unicons/"
)

var canvasIDInvalid = regexp.MustCompile(`[^A-Za-z0-9_-]`)

type canvasBox struct{ left, top, width, height float64 }

type canvasLinks struct {
	from  string
	path  string
	items []any
}

type canvasBuild struct {
	b        *panelBuild
	d        *spec.Display
	defaults spec.CanvasElementDefaults
	width    float64
	height   float64
	names    map[string]string
	ids      map[string]bool
	links    []canvasLinks
}

func displayCanvas(b *panelBuild) {
	const root = "options.root"
	d := b.disp()
	c := &canvasBuild{
		b: b, d: d, defaults: spec.Defaults().CanvasElement,
		width:  float64(b.n.pos.w * canvasColumnPixels),
		height: max(float64(b.n.pos.h*canvasRowPixels-canvasRowGapPixels-canvasHeaderPixels), 1),
		names:  map[string]string{}, ids: map[string]bool{},
	}
	c.root(root)
	items, _ := asArray(lookupValue(b.raw, root+".elements"))
	c.walk(items, root+".elements", canvasBox{0, 0, 100, 100})
	c.connections()
}

func (c *canvasBuild) root(root string) {
	c.b.consume(root+".type", root+".name", root+".constraint", root+".placement")
	c.d.PanZoom, _ = c.b.boolOption("options.panZoom")
	c.d.InfinitePan, _ = c.b.boolOption("options.infinitePan")
	c.rootTooltip()
	el, _ := asObject(lookupValue(c.b.raw, root))
	c.d.BackgroundColor = c.color(el, root, "background.color", "")
	c.d.BackgroundImage, c.d.BackgroundSize = c.backgroundImage(el, root)
	if width, ok := c.borderWidth(el, root); ok && width > 0 {
		c.d.BorderWidth = &width
		c.d.BorderColor = c.color(el, root, "border.color", "")
	}
	c.d.BorderRadius = c.radius(el, root)
	c.b.consume(root + ".border")
}

func (c *canvasBuild) rootTooltip() {
	switch asString(lookupValue(c.b.raw, "options.tooltip.mode")) {
	case "", "details":
		c.d.Tooltip = "details"
		c.b.consume("options.tooltip.mode")
	case "none":
		c.d.Tooltip = "none"
		c.b.consume("options.tooltip.mode")
	}
}

func (c *canvasBuild) walk(items []any, base string, parent canvasBox) {
	for i, item := range items {
		el, ok := asObject(item)
		if !ok {
			continue
		}
		path := fmt.Sprintf("%s[%d]", base, i)
		switch typ := asString(el["type"]); typ {
		case "frame":
			c.frame(el, path, parent)
		default:
			if _, known := canvasKinds[typ]; known {
				c.element(el, path, typ, parent)
				continue
			}
			c.b.partial(path, fmt.Sprintf(canvasTypeReason, typ))
		}
	}
}

func (c *canvasBuild) frame(el map[string]any, path string, parent canvasBox) {
	box, _, ok := c.place(el, path, parent)
	c.b.partial(path+".type", canvasFrameReason)
	c.b.consume(path+".name", path+".constraint", path+".placement", path+".background", path+".border", path+".config")
	if !ok {
		c.b.partial(path+".elements", canvasPlacementReason)
		return
	}
	children, _ := asArray(el["elements"])
	c.walk(children, path+".elements", box)
}

func (c *canvasBuild) element(el map[string]any, path, typ string, parent canvasBox) {
	if len(c.d.Elements) >= canvasElementLimit {
		c.b.partial(path, canvasElementsReason)
		return
	}
	box, placement, ok := c.place(el, path, parent)
	if !ok {
		c.b.partial(path, canvasPlacementReason)
		return
	}
	name := asString(el["name"])
	out := spec.CanvasElement{ID: c.uniqueID(name), Kind: canvasKind(el, typ), Left: box.left, Top: box.top, Width: box.width, Height: box.height, Placement: placement}
	c.b.consume(path+".type", path+".name")
	c.text(&out, el, path, typ)
	c.look(&out, el, path)
	c.kindSettings(&out, el, path)
	c.elementLinks(&out, el, path)
	c.elementActions(&out, el, path)
	c.d.Elements = append(c.d.Elements, out)
	if _, taken := c.names[name]; !taken && name != "" {
		c.names[name] = out.ID
	}
	if items, _ := asArray(el["connections"]); len(items) > 0 {
		c.links = append(c.links, canvasLinks{from: out.ID, path: path + ".connections", items: items})
	}
}

var canvasKinds = map[string]string{
	"metric-value":  "metric",
	"text":          "text",
	"rectangle":     "rectangle",
	"ellipse":       "ellipse",
	"icon":          "icon",
	"button":        "button",
	"server":        "server",
	"triangle":      "triangle",
	"cloud":         "cloud",
	"parallelogram": "parallelogram",
	"windTurbine":   "windTurbine",
	"droneTop":      "droneTop",
	"droneFront":    "droneFront",
	"droneSide":     "droneSide",
}

var canvasServerKinds = []string{"single", "stack", "database", "terminal"}

var canvasButtonStyles = []string{"primary", "secondary", "success", "destructive"}

var canvasAngleFields = map[string]string{
	"windTurbine": "config.rpm.field",
	"droneTop":    "config.yawAngle.field",
	"droneFront":  "config.rollAngle.field",
	"droneSide":   "config.pitchAngle.field",
}

func canvasKind(el map[string]any, typ string) string {
	if typ == "text" && asString(lookupValue(el, "config.text.mode")) == "field" {
		return "metric"
	}
	return canvasKinds[typ]
}

func (c *canvasBuild) kindSettings(out *spec.CanvasElement, el map[string]any, path string) {
	switch out.Kind {
	case "icon":
		c.icon(out, el, path)
	case "server":
		c.server(out, el, path)
	case "button":
		out.ButtonStyle = c.b.enumOption(path+".config.style.variant", canvasButtonStyles...)
		c.buttonAction(out, el, path)
	case "windTurbine":
		out.RateField = c.b.fieldNameOption(path + ".config.rpm.field")
	case "droneTop":
		out.YawField = c.b.fieldNameOption(path + ".config.yawAngle.field")
	case "droneFront":
		out.RollField = c.b.fieldNameOption(path + ".config.rollAngle.field")
	case "droneSide":
		out.PitchField = c.b.fieldNameOption(path + ".config.pitchAngle.field")
	}
}

func (c *canvasBuild) server(out *spec.CanvasElement, el map[string]any, path string) {
	if kind := strings.ToLower(asString(lookupValue(el, "config.type"))); slices.Contains(canvasServerKinds, kind) {
		out.Server = kind
		c.b.consume(path + ".config.type")
	}
	out.AccentColor = c.color(el, path, "config.statusColor", "")
}

func (c *canvasBuild) icon(out *spec.CanvasElement, el map[string]any, path string) {
	base := path + ".config.path"
	cfg, _ := asObject(lookupValue(el, "config.path"))
	c.b.consume(base)
	switch mode := asString(cfg["mode"]); mode {
	case "field":
		if field := asString(cfg["field"]); field != "" && validLength(field, maxFieldName) {
			out.Image = &spec.ImageRef{Field: field}
		} else {
			c.b.issues.add(base+".field", canvasIconReason)
		}
	case "", "fixed":
		c.fixedIcon(out, base+".fixed", strings.TrimSpace(asString(cfg["fixed"])))
	default:
		c.b.issues.add(base+".mode", canvasIconReason)
	}
	out.IconColor = c.color(el, path, "config.fill", "")
}

var canvasIconName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

func (c *canvasBuild) fixedIcon(out *spec.CanvasElement, at, value string) {
	name := strings.TrimSuffix(strings.TrimPrefix(value, bundledIconPrefix), ".svg")
	switch {
	case value == "":
	case strings.HasPrefix(value, bundledIconPrefix) && canvasIconName.MatchString(name):
		out.Icon = name
	case literalHTTPS(value):
		out.Image = &spec.ImageRef{URL: value}
	default:
		c.b.issues.add(at, canvasIconReason)
	}
}

func (c *canvasBuild) uniqueID(name string) string {
	id := canvasIDInvalid.ReplaceAllString(name, "_")
	if id == "" || !isLetter(id[0]) {
		id = canvasUnnamedElement + id
	}
	id, _ = cutRunes(id, canvasIDLimit)
	unique := id
	for n := 2; c.ids[unique]; n++ {
		suffix := fmt.Sprintf("_%d", n)
		base, _ := cutRunes(id, canvasIDLimit-len(suffix))
		unique = base + suffix
	}
	c.ids[unique] = true
	return unique
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func optionalNumber(v any) *float64 {
	f, ok := numberValue(v)
	if !ok {
		return nil
	}
	return &f
}

func axisPlace(kind string, start, end, size *float64, total float64) (pos, length float64, ok bool) {
	switch {
	case kind == "center" && start != nil && size != nil:
		return total/2 + *start - *size/2, *size, true
	case kind == "end" && end != nil && size != nil:
		return total - *end - *size, *size, true
	case kind == "both" && start != nil && end != nil:
		return *start, total - *start - *end, true
	case start != nil && size != nil:
		return *start, *size, true
	case end != nil && size != nil:
		return total - *end - *size, *size, true
	case start != nil && end != nil:
		return *start, total - *start - *end, true
	}
	return 0, 0, false
}

func roundPercent(v float64) float64 {
	return math.Round(v*100) / 100
}

var (
	canvasHorizontalModes = []string{"left", "right", "leftright", "center", "scale"}
	canvasVerticalModes   = []string{"top", "bottom", "topbottom", "center", "scale"}
)

func constraintMode(raw any, modes []string) string {
	if mode := asString(raw); slices.Contains(modes, mode) {
		return mode
	}
	return modes[0]
}

var canvasAxisKinds = map[string]string{
	"left": "start", "top": "start", "right": "end", "bottom": "end",
	"leftright": "both", "topbottom": "both", "center": "center", "scale": "scale",
}

type canvasAxis struct {
	kind              string
	start, end, size  *float64
	pos, length, base float64
}

func (c *canvasBuild) axis(mode string, start, end, size *float64, extent float64) (canvasAxis, bool) {
	a := canvasAxis{kind: canvasAxisKinds[mode], start: start, end: end, size: size, base: extent}
	if a.kind == "scale" {
		a.base = 100
	}
	var ok bool
	a.pos, a.length, ok = axisPlace(a.kind, start, end, size, a.base)
	return a, ok && a.length > 0
}

func (a canvasAxis) percent() (pos, length float64) {
	return a.pos / a.base * 100, a.length / a.base * 100
}

func (c *canvasBuild) place(el map[string]any, path string, parent canvasBox) (canvasBox, *spec.CanvasPlacement, bool) {
	placement, _ := asObject(el["placement"])
	constraint, _ := asObject(el["constraint"])
	hMode := constraintMode(constraint["horizontal"], canvasHorizontalModes)
	vMode := constraintMode(constraint["vertical"], canvasVerticalModes)
	x, okX := c.axis(hMode, optionalNumber(placement["left"]), optionalNumber(placement["right"]), optionalNumber(placement["width"]), parent.width/100*c.width)
	y, okY := c.axis(vMode, optionalNumber(placement["top"]), optionalNumber(placement["bottom"]), optionalNumber(placement["height"]), parent.height/100*c.height)
	c.b.consume(path+".constraint", path+".placement.left", path+".placement.right", path+".placement.top", path+".placement.bottom", path+".placement.width", path+".placement.height")
	if !okX || !okY {
		return canvasBox{}, nil, false
	}
	inBody := parent == canvasBox{0, 0, 100, 100}
	scaled := hMode == "scale" && vMode == "scale"
	if !inBody && !scaled {
		c.b.partial(path+".placement", canvasPixelReason)
	}
	px, pw := x.percent()
	py, ph := y.percent()
	px, py = min(max(px, 0), 100), min(max(py, 0), 100)
	pw, ph = min(max(pw, 0.01), 100), min(max(ph, 0.01), 100)
	box := canvasBox{
		left:   roundPercent(parent.left + px*parent.width/100),
		top:    roundPercent(parent.top + py*parent.height/100),
		width:  roundPercent(max(pw*parent.width/100, 0.01)),
		height: roundPercent(max(ph*parent.height/100, 0.01)),
	}
	if !inBody || scaled {
		return box, nil, true
	}
	return box, &spec.CanvasPlacement{
		Horizontal: hMode, Vertical: vMode,
		Left: x.startValue(), Right: x.endValue(), Width: x.sizeValue(),
		Top: y.startValue(), Bottom: y.endValue(), Height: y.sizeValue(),
	}, true
}

func (a canvasAxis) startValue() *float64 {
	if a.kind == "end" {
		return nil
	}
	return a.start
}

func (a canvasAxis) endValue() *float64 {
	if a.kind == "end" || a.kind == "both" {
		return a.end
	}
	return nil
}

func (a canvasAxis) sizeValue() *float64 {
	if a.kind == "both" {
		return nil
	}
	return a.size
}

func (c *canvasBuild) text(out *spec.CanvasElement, el map[string]any, path, typ string) {
	cfg, _ := asObject(lookupValue(el, "config.text"))
	mode := asString(cfg["mode"])
	base := path + ".config.text"
	c.b.consume(base)
	if mode == "template" {
		c.b.partial(base+".mode", canvasTemplateReason)
	}
	if typ == "metric-value" || mode == "field" {
		field := asString(cfg["field"])
		switch {
		case field == "":
		case !validLength(field, maxFieldName):
			c.b.partial(base+".field", fieldNameLenReason)
		default:
			out.Field = field
		}
		return
	}
	text, _ := cutRunes(strings.TrimSpace(asString(cfg["fixed"])), canvasTextLimit)
	out.Text = text
}

func (c *canvasBuild) look(out *spec.CanvasElement, el map[string]any, path string) {
	out.Fill = c.color(el, path, "background.color", out.Field)
	out.TextColor = c.color(el, path, "config.color", out.Field)
	c.border(out, el, path)
	out.BackgroundImage, out.BackgroundSize = c.backgroundImage(el, path)
	c.fontAndAlign(out, el, path)
	if valign := c.b.enumOption(path+".config.valign", "top", "middle", "bottom"); valign != c.defaults.VAlign {
		out.VAlign = valign
	}
	out.Rotation = c.rotation(el, path)
}

func (c *canvasBuild) rotation(el map[string]any, path string) *float64 {
	at := path + ".placement.rotation"
	rotation := optionalNumber(lookupValue(el, "placement.rotation"))
	c.b.consume(at)
	switch {
	case rotation == nil || *rotation == 0:
		return nil
	case math.Abs(*rotation) > canvasMaxRotation:
		c.b.issues.add(at, canvasRotationReason)
		return nil
	}
	return rotation
}

func (c *canvasBuild) backgroundImage(el map[string]any, path string) (*spec.ImageRef, string) {
	base := path + ".background"
	cfg, _ := asObject(lookupValue(el, "background.image"))
	if cfg == nil {
		return nil, ""
	}
	c.b.consume(base + ".image")
	var image *spec.ImageRef
	switch mode := asString(cfg["mode"]); mode {
	case "", "fixed":
		image = c.fixedImage(base+".image.fixed", strings.TrimSpace(asString(cfg["fixed"])))
	case "field":
		if field := asString(cfg["field"]); field != "" && validLength(field, maxFieldName) {
			image = &spec.ImageRef{Field: field}
		} else {
			c.b.issues.add(base+".image.field", canvasImageReason)
		}
	default:
		c.b.issues.add(base+".image.mode", canvasImageModeReason)
	}
	return image, c.b.enumOption(base+".size", "original", "contain", "cover", "fill", "tile")
}

func (c *canvasBuild) fixedImage(at, value string) *spec.ImageRef {
	switch {
	case value == "":
		return nil
	case literalHTTPS(value):
		return &spec.ImageRef{URL: value}
	}
	c.b.issues.add(at, canvasImageReason)
	return nil
}

func (c *canvasBuild) color(el map[string]any, path, rel, ownField string) string {
	cfg, _ := asObject(lookupValue(el, rel))
	full := path + "." + rel
	c.b.consume(full)
	if field := asString(cfg["field"]); field != "" {
		if field == ownField {
			return "field"
		}
		c.b.partial(full+".field", canvasFieldReason)
	}
	raw := strings.TrimSpace(asString(cfg["fixed"]))
	if raw == "" || strings.EqualFold(raw, "transparent") {
		return ""
	}
	color, ok := seriesColor(raw)
	if !ok {
		c.b.partial(full+".fixed", fmt.Sprintf("The color %q is not supported yet.", raw))
		return ""
	}
	return color
}

func (c *canvasBuild) border(out *spec.CanvasElement, el map[string]any, path string) {
	if width, ok := c.borderWidth(el, path); ok && width > 0 {
		out.Stroke = c.color(el, path, "border.color", out.Field)
		if width != c.defaults.BorderWidth {
			out.BorderWidth = &width
		}
	}
	c.b.consume(path + ".border.color")
	if radius := c.radius(el, path); radius != nil && *radius != c.defaults.Radius {
		out.Radius = radius
	}
}

func (c *canvasBuild) borderWidth(el map[string]any, path string) (int, bool) {
	at := path + ".border.width"
	raw := optionalNumber(lookupValue(el, "border.width"))
	c.b.consume(at)
	if raw == nil {
		return 0, false
	}
	width := int(math.Round(*raw))
	if width < 0 || width > canvasMaxBorderWidth {
		c.b.issues.add(at, canvasBorderReason)
		return 0, false
	}
	return width, true
}

func (c *canvasBuild) radius(el map[string]any, path string) *int {
	at := path + ".border.radius"
	raw := optionalNumber(lookupValue(el, "border.radius"))
	c.b.consume(at)
	if raw == nil {
		return nil
	}
	radius := int(math.Round(*raw))
	if radius < 0 || radius > canvasMaxRadius {
		c.b.issues.add(at, canvasRadiusReason)
		return nil
	}
	return &radius
}

func (c *canvasBuild) fontAndAlign(out *spec.CanvasElement, el map[string]any, path string) {
	c.b.consume(path+".config.size", path+".config.align")
	if size := optionalNumber(lookupValue(el, "config.size")); size != nil {
		n := int(math.Round(*size))
		if n < canvasMinFontSize || n > canvasMaxFontSize {
			c.b.partial(path+".config.size", canvasFontReason)
			n = min(max(n, canvasMinFontSize), canvasMaxFontSize)
		}
		if n != c.defaults.FontSize {
			out.FontSize = n
		}
	}
	switch align := asString(lookupValue(el, "config.align")); align {
	case "":
	case "left", "center", "right":
		if align != c.defaults.Align {
			out.Align = align
		}
	default:
		c.b.partial(path+".config.align", canvasAlignReason)
	}
}

func (c *canvasBuild) connections() {
	defaultWidth := spec.Defaults().CanvasConnection.Width
	for _, group := range c.links {
		for j, item := range group.items {
			path := fmt.Sprintf("%s[%d]", group.path, j)
			conn, _ := asObject(item)
			target := c.names[asString(conn["targetName"])]
			switch {
			case target == "" || target == group.from:
				c.b.partial(path, canvasConnectionReason)
			case len(c.d.Connections) >= canvasConnectionMax:
				c.b.partial(path, canvasConnLimitReason)
			default:
				c.d.Connections = append(c.d.Connections, c.connection(conn, path, group.from, target, defaultWidth))
			}
		}
	}
}

func (c *canvasBuild) connection(conn map[string]any, path, from, to string, defaultWidth int) spec.CanvasConnection {
	out := spec.CanvasConnection{From: from, To: to}
	if field := asString(lookupValue(conn, "color.field")); field != "" && validLength(field, maxFieldName) {
		out.Field, out.Color = field, "field"
		c.b.consume(path + ".color")
	} else {
		out.Color = c.color(conn, path, "color", "")
	}
	c.connectionWidth(&out, conn, path, defaultWidth)
	if asString(conn["path"]) == "curved" {
		out.Path = "curved"
	}
	c.b.consume(path + ".path")
	if style := asString(lookupValue(conn, "lineStyle.style")); style == "dashed" || style == "dotted" {
		out.LineStyle = style
	}
	c.connectionArrow(&out, conn, path)
	if present(conn["vertices"]) {
		c.b.partial(path+".vertices", canvasVerticesReason)
	}
	c.b.consume(path+".lineStyle", path+".source", path+".target", path+".sourceOriginal", path+".targetOriginal", path+".targetName")
	return out
}

func (c *canvasBuild) connectionWidth(out *spec.CanvasConnection, conn map[string]any, path string, defaultWidth int) {
	if field := asString(lookupValue(conn, "size.field")); field != "" && validLength(field, maxFieldName) {
		out.WidthField = field
		out.MinWidth = c.connectionBound(conn, path+".size.min")
		out.MaxWidth = c.connectionBound(conn, path+".size.max")
		c.b.consume(path+".size.field", path+".size.fixed")
	} else if v, ok := lookup(conn, "size.fixed"); ok {
		f, isNumber := numberValue(v)
		n := int(math.Round(f))
		if !isNumber || n < canvasMinConnWidth || n > canvasMaxConnWidth {
			c.b.partial(path+".size.fixed", canvasConnSizeReason)
		} else if n != defaultWidth {
			out.Width = n
		}
		c.b.consume(path+".size.fixed", path+".size.min", path+".size.max")
	}
	if mode := asString(lookupValue(conn, "size.mode")); mode != "" {
		c.b.partial(path+".size.mode", canvasConnModeReason)
	}
}

func (c *canvasBuild) connectionBound(conn map[string]any, at string) int {
	v, ok := c.b.take(at)
	if !ok {
		return 0
	}
	f, isNumber := numberValue(v)
	n := int(math.Round(f))
	if !isNumber || n < canvasMinConnWidth || n > canvasMaxConnWidth {
		c.b.issues.add(at, canvasConnSizeReason)
		return 0
	}
	return n
}

func (c *canvasBuild) connectionArrow(out *spec.CanvasConnection, conn map[string]any, path string) {
	if asString(lookupValue(conn, "direction.mode")) == "field" {
		c.b.partial(path+".direction", canvasConnModeReason)
		return
	}
	switch direction := asString(lookupValue(conn, "direction.fixed")); direction {
	case "forward", "reverse", "both":
		out.Arrow = direction
		c.b.consume(path + ".direction")
	case "none":
		c.b.consume(path + ".direction")
	}
}
