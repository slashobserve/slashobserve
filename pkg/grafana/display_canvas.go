package grafana

import (
	"fmt"
	"math"
	"regexp"
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
	canvasPixelReason      = "Element positions were converted to percentages of the panel and may shift."
	canvasPlacementReason  = "A canvas element needs a position and a size; it was left out."
	canvasRotationReason   = "Rotated elements are drawn upright."
	canvasTemplateReason   = "Text templates are not supported; the template is shown as plain text."
	canvasFieldReason      = "A color taken from a field other than the element's own is not supported."
	canvasBorderReason     = "Only 1 pixel borders are supported."
	canvasRadiusReason     = "Rounded borders are not supported."
	canvasImageReason      = "Background images are not supported."
	canvasValignReason     = "Text is centered vertically."
	canvasLinksReason      = "Links on canvas elements are not supported."
	canvasActionsReason    = "Canvas actions are never imported."
	canvasFontReason       = "Only font sizes from 8 to 72 are supported."
	canvasAlignReason      = "Only left, center and right alignment is supported."
	canvasElementsReason   = "A canvas holds at most 50 elements."
	canvasBackgroundReason = "The canvas background is not drawn."
	canvasConnectionReason = "A connection to an element that was left out, or to itself, is not kept."
	canvasConnLimitReason  = "A canvas holds at most 100 connections."
	canvasCurvedReason     = "Curved connections are drawn straight."
	canvasLineStyleReason  = "Only solid connection lines are supported."
	canvasConnSizeReason   = "Only fixed connection widths from 1 to 10 are supported."
	canvasConnFieldReason  = "Connection widths taken from a field are not supported."
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
	b.consume(root+".type", root+".name", root+".constraint", root+".placement", root+".border")
	if fixed := strings.ToLower(asString(lookupValue(b.raw, root+".background.color.fixed"))); fixed != "" && fixed != "transparent" {
		b.partial(root+".background", canvasBackgroundReason)
	}
	b.consume(root + ".background")
	items, _ := asArray(lookupValue(b.raw, root+".elements"))
	c.walk(items, root+".elements", canvasBox{0, 0, 100, 100})
	c.connections()
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
		case "metric-value", "text", "rectangle", "ellipse":
			c.element(el, path, typ, parent)
		default:
			c.b.partial(path, fmt.Sprintf(canvasTypeReason, typ))
		}
	}
}

func (c *canvasBuild) frame(el map[string]any, path string, parent canvasBox) {
	box, ok := c.place(el, path, parent)
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
	box, ok := c.place(el, path, parent)
	if !ok {
		c.b.partial(path, canvasPlacementReason)
		return
	}
	name := asString(el["name"])
	out := spec.CanvasElement{ID: c.uniqueID(name), Kind: canvasKind(el, typ), Left: box.left, Top: box.top, Width: box.width, Height: box.height}
	c.b.consume(path+".type", path+".name")
	c.text(&out, el, path, typ)
	c.look(&out, el, path)
	c.d.Elements = append(c.d.Elements, out)
	if _, taken := c.names[name]; !taken && name != "" {
		c.names[name] = out.ID
	}
	if items, _ := asArray(el["connections"]); len(items) > 0 {
		c.links = append(c.links, canvasLinks{from: out.ID, path: path + ".connections", items: items})
	}
	if present(el["links"]) {
		c.b.partial(path+".links", canvasLinksReason)
	}
	if present(el["actions"]) {
		c.b.partial(path+".actions", canvasActionsReason)
	}
}

func canvasKind(el map[string]any, typ string) string {
	switch typ {
	case "metric-value":
		return "metric"
	case "text":
		if asString(lookupValue(el, "config.text.mode")) == "field" {
			return "metric"
		}
		return "text"
	}
	return typ
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

func axisPlace(start, end, size *float64, total float64) (pos, length float64, ok bool) {
	switch {
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

func (c *canvasBuild) place(el map[string]any, path string, parent canvasBox) (canvasBox, bool) {
	placement, _ := asObject(el["placement"])
	constraint, _ := asObject(el["constraint"])
	scaled := asString(constraint["horizontal"]) == "scale" && asString(constraint["vertical"]) == "scale"
	totalW, totalH := 100.0, 100.0
	if !scaled {
		totalW, totalH = parent.width/100*c.width, parent.height/100*c.height
	}
	x, w, okX := axisPlace(optionalNumber(placement["left"]), optionalNumber(placement["right"]), optionalNumber(placement["width"]), totalW)
	y, h, okY := axisPlace(optionalNumber(placement["top"]), optionalNumber(placement["bottom"]), optionalNumber(placement["height"]), totalH)
	if rotation := optionalNumber(placement["rotation"]); rotation != nil && *rotation != 0 {
		c.b.partial(path+".placement.rotation", canvasRotationReason)
	}
	c.b.consume(path+".constraint", path+".placement")
	if !okX || !okY || w <= 0 || h <= 0 {
		return canvasBox{}, false
	}
	if !scaled {
		c.b.partial(path+".placement", canvasPixelReason)
		x, w, y, h = x/totalW*100, w/totalW*100, y/totalH*100, h/totalH*100
	}
	x, y = min(max(x, 0), 100), min(max(y, 0), 100)
	w, h = min(max(w, 0.01), 100), min(max(h, 0.01), 100)
	return canvasBox{
		left:   roundPercent(parent.left + x*parent.width/100),
		top:    roundPercent(parent.top + y*parent.height/100),
		width:  roundPercent(max(w*parent.width/100, 0.01)),
		height: roundPercent(max(h*parent.height/100, 0.01)),
	}, true
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
	if present(lookupValue(el, "background.image.fixed")) {
		c.b.partial(path+".background.image", canvasImageReason)
	}
	c.b.consume(path + ".background.image")
	c.fontAndAlign(out, el, path)
	if valign := asString(lookupValue(el, "config.valign")); valign != "" && valign != "middle" {
		c.b.partial(path+".config.valign", canvasValignReason)
	}
	c.b.consume(path + ".config.valign")
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
	width := optionalNumber(lookupValue(el, "border.width"))
	if width != nil && *width > 0 {
		out.Stroke = c.color(el, path, "border.color", out.Field)
		if *width != 1 {
			c.b.partial(path+".border.width", canvasBorderReason)
		}
	}
	c.b.consume(path + ".border.color")
	if radius := optionalNumber(lookupValue(el, "border.radius")); radius != nil && *radius > 0 {
		c.b.partial(path+".border.radius", canvasRadiusReason)
	}
	c.b.consume(path+".border.width", path+".border.radius")
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
	if v, ok := lookup(conn, "size.fixed"); ok {
		f, isNumber := numberValue(v)
		n := int(math.Round(f))
		if !isNumber || n < canvasMinConnWidth || n > canvasMaxConnWidth {
			c.b.partial(path+".size.fixed", canvasConnSizeReason)
		} else if n != defaultWidth {
			out.Width = n
		}
	}
	if present(lookupValue(conn, "size.field")) {
		c.b.partial(path+".size.field", canvasConnFieldReason)
	}
	if asString(conn["path"]) == "curved" {
		c.b.partial(path+".path", canvasCurvedReason)
	}
	if style := asString(lookupValue(conn, "lineStyle.style")); style != "" && style != "solid" {
		c.b.partial(path+".lineStyle.style", canvasLineStyleReason)
	}
	c.b.consume(path+".size", path+".path", path+".lineStyle", path+".source", path+".target", path+".vertices",
		path+".sourceOriginal", path+".targetOriginal", path+".targetName")
	return out
}
