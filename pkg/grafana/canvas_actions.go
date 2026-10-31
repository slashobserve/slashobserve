package grafana

import (
	"fmt"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	canvasLinkMax   = 10
	canvasActionMax = 10

	canvasLinkLimitReason   = "A canvas element holds at most 10 links."
	canvasActionLimitReason = "A canvas element holds at most 10 actions."
)

func (c *canvasBuild) elementLinks(out *spec.CanvasElement, el map[string]any, path string) {
	items, _ := asArray(el["links"])
	oneClick := false
	for i, item := range items {
		at := fmt.Sprintf("%s.links[%d]", path, i)
		link, reason := linkOf(item)
		switch {
		case reason != "":
			c.b.partial(at, reason)
		case len(out.Links) >= canvasLinkMax:
			c.b.partial(at, canvasLinkLimitReason)
		default:
			oneClick = oneClick || link.OneClick
			link.OneClick = false
			out.Links = append(out.Links, link)
			c.b.consume(at)
		}
	}
	if oneClick {
		out.OneClick = "link"
	}
}

func (c *canvasBuild) elementActions(out *spec.CanvasElement, el map[string]any, path string) {
	items, _ := asArray(el["actions"])
	oneClick := false
	for i, item := range items {
		at := fmt.Sprintf("%s.actions[%d]", path, i)
		raw, _ := asObject(item)
		action, ok := c.b.fetchAction(at, at, raw, false)
		switch {
		case !ok:
		case len(out.Actions) >= canvasActionMax:
			c.b.partial(at, canvasActionLimitReason)
		default:
			out.Actions = append(out.Actions, action)
			oneClick = oneClick || truthy(raw["oneClick"])
		}
	}
	if oneClick && out.OneClick == "" {
		out.OneClick = "action"
	} else if oneClick {
		c.b.issues.add(path+".actions", oneClickTargetReason)
	}
}

func (c *canvasBuild) buttonAction(out *spec.CanvasElement, el map[string]any, path string) {
	api, ok := asObject(lookupValue(el, "config.api"))
	if !ok || asString(api["endpoint"]) == "" {
		return
	}
	at := path + ".config.api"
	c.b.consume(at)
	parts := actionParts{
		title:       out.Text,
		method:      asString(api["method"]),
		url:         asString(api["endpoint"]),
		contentType: asString(api["contentType"]),
		body:        asString(api["data"]),
		headers:     c.b.actionParams(at+".headerParams", api["headerParams"]),
		query:       c.b.actionParams(at+".queryParams", api["queryParams"]),
	}
	if action, valid := c.b.finishAction(at, parts); valid && len(out.Actions) < canvasActionMax {
		out.Actions = append(out.Actions, action)
	}
}
