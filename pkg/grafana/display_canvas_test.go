package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const canvasScaled = `"constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10}`

func canvasDocPanel(t *testing.T, templating, options, elements string) (spec.Panel, PanelLine) {
	t.Helper()
	doc := fmt.Sprintf(`{"title":"T","schemaVersion":39,%s"panels":[{"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":12,"h":10},"datasource":{"type":"prometheus","uid":"prom"},"type":"canvas",%s,"options":{%s"root":{"type":"frame","name":"root","elements":[%s]}}}]}`,
		templating, upTarget, options, elements)
	return convertedPanel(t, doc)
}

func oneCanvasElement(t *testing.T, element string) (spec.CanvasElement, PanelLine) {
	t.Helper()
	panel, line := canvasPanel(t, 12, 10, element)
	els := canvasElements(t, panel)
	if len(els) != 1 {
		t.Fatalf("elements = %+v, paths %v", els, line.Paths)
	}
	return els[0], line
}

func TestCanvasElementTypes(t *testing.T) {
	cases := []struct {
		name, element string
		check         func(spec.CanvasElement) bool
	}{
		{"bundled icon", `{"type":"icon","name":"i",` + canvasScaled + `,"config":{"path":{"mode":"fixed","fixed":"img/icons/unicons/bolt.svg"},"fill":{"fixed":"red"}}}`,
			func(e spec.CanvasElement) bool {
				return e.Kind == "icon" && e.Icon == "bolt" && e.IconColor == "#F2495C"
			}},
		{"icon from an https image", `{"type":"icon","name":"i",` + canvasScaled + `,"config":{"path":{"mode":"fixed","fixed":"https://example.com/a.svg"}}}`,
			func(e spec.CanvasElement) bool { return e.Image != nil && e.Image.URL == "https://example.com/a.svg" }},
		{"icon from a field", `{"type":"icon","name":"i",` + canvasScaled + `,"config":{"path":{"mode":"field","field":"img"}}}`,
			func(e spec.CanvasElement) bool { return e.Image != nil && e.Image.Field == "img" }},
		{"server", `{"type":"server","name":"s",` + canvasScaled + `,"config":{"type":"Database","statusColor":{"fixed":"red"}}}`,
			func(e spec.CanvasElement) bool {
				return e.Kind == "server" && e.Server == "database" && e.AccentColor == "#F2495C"
			}},
		{"button", `{"type":"button","name":"b",` + canvasScaled + `,"config":{"text":{"mode":"fixed","fixed":"Restart"},"style":{"variant":"destructive"}}}`,
			func(e spec.CanvasElement) bool {
				return e.Kind == "button" && e.ButtonStyle == "destructive" && e.Text == "Restart"
			}},
		{"wind turbine", `{"type":"windTurbine","name":"w",` + canvasScaled + `,"config":{"rpm":{"field":"rpm"}}}`,
			func(e spec.CanvasElement) bool { return e.Kind == "windTurbine" && e.RateField == "rpm" }},
		{"drone top", `{"type":"droneTop","name":"d",` + canvasScaled + `,"config":{"yawAngle":{"field":"yaw"}}}`,
			func(e spec.CanvasElement) bool { return e.Kind == "droneTop" && e.YawField == "yaw" }},
		{"drone front", `{"type":"droneFront","name":"d",` + canvasScaled + `,"config":{"rollAngle":{"field":"roll"}}}`,
			func(e spec.CanvasElement) bool { return e.Kind == "droneFront" && e.RollField == "roll" }},
		{"drone side", `{"type":"droneSide","name":"d",` + canvasScaled + `,"config":{"pitchAngle":{"field":"pitch"}}}`,
			func(e spec.CanvasElement) bool { return e.Kind == "droneSide" && e.PitchField == "pitch" }},
		{"triangle", `{"type":"triangle","name":"t",` + canvasScaled + `}`, func(e spec.CanvasElement) bool { return e.Kind == "triangle" }},
		{"cloud", `{"type":"cloud","name":"t",` + canvasScaled + `}`, func(e spec.CanvasElement) bool { return e.Kind == "cloud" }},
		{"parallelogram", `{"type":"parallelogram","name":"t",` + canvasScaled + `}`, func(e spec.CanvasElement) bool { return e.Kind == "parallelogram" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el, line := oneCanvasElement(t, tc.element)
			if !tc.check(el) {
				t.Errorf("element = %+v", el)
			}
			if line.Status != statusImported {
				t.Errorf("paths = %v, reasons = %v", line.Paths, line.Reasons)
			}
		})
	}
}

func TestCanvasIconThatCannotBeLoadedIsReported(t *testing.T) {
	el, line := oneCanvasElement(t, `{"type":"icon","name":"i",`+canvasScaled+`,"config":{"path":{"mode":"fixed","fixed":"public/img/custom.svg"}}}`)
	if el.Icon != "" || el.Image != nil || !slices.Contains(line.Paths, "options.root.elements[0].config.path.fixed") {
		t.Fatalf("element = %+v, paths = %v", el, line.Paths)
	}
}

func TestCanvasElementStyling(t *testing.T) {
	el, line := oneCanvasElement(t, `{"type":"rectangle","name":"m","constraint":{"horizontal":"scale","vertical":"scale"},"placement":{"left":0,"top":0,"width":10,"height":10,"rotation":45},`+
		`"config":{"valign":"top"},"background":{"image":{"mode":"fixed","fixed":"https://example.com/bg.png"},"size":"cover"},"border":{"width":3,"radius":8,"color":{"fixed":"red"}}}`)
	if el.Rotation == nil || *el.Rotation != 45 || el.VAlign != "top" || el.BorderWidth == nil || *el.BorderWidth != 3 || el.Radius == nil || *el.Radius != 8 {
		t.Errorf("element = %+v", el)
	}
	if el.BackgroundImage == nil || el.BackgroundImage.URL != "https://example.com/bg.png" || el.BackgroundSize != "cover" || el.Stroke != "#F2495C" {
		t.Errorf("element = %+v", el)
	}
	if line.Status != statusImported {
		t.Errorf("paths = %v", line.Paths)
	}
}

func TestCanvasBackgroundImageFromAFieldAndDefaultsStaySilent(t *testing.T) {
	el, line := oneCanvasElement(t, `{"type":"rectangle","name":"m",`+canvasScaled+`,"config":{"valign":"middle"},"background":{"image":{"mode":"field","field":"pic"}},"border":{"width":1,"radius":0}}`)
	if el.BackgroundImage == nil || el.BackgroundImage.Field != "pic" || el.VAlign != "" || el.BorderWidth != nil || el.Radius != nil {
		t.Errorf("element = %+v", el)
	}
	if line.Status != statusImported {
		t.Errorf("paths = %v", line.Paths)
	}
}

func TestCanvasRootOptions(t *testing.T) {
	panel, line := canvasDocPanel(t, "", `"panZoom":true,"infinitePan":true,"tooltip":{"mode":"none"},`, "")
	d := displayOf(t, panel)
	if d.PanZoom == nil || !*d.PanZoom || d.InfinitePan == nil || !*d.InfinitePan || d.Tooltip != "" || line.Status != statusImported {
		t.Errorf("display = %+v paths %v", d, line.Paths)
	}
	panel, _ = canvasDocPanel(t, "", "", "")
	if got := displayOf(t, panel).Tooltip; got != "details" {
		t.Errorf("tooltip = %q, want details", got)
	}
}

func TestCanvasRootBackgroundAndBorder(t *testing.T) {
	doc := fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":12,"h":10},"datasource":{"type":"prometheus","uid":"prom"},"type":"canvas",%s,`+
		`"options":{"root":{"type":"frame","name":"root","background":{"color":{"fixed":"blue"},"image":{"mode":"fixed","fixed":"https://example.com/bg.png"},"size":"contain"},"border":{"width":2,"radius":6,"color":{"fixed":"red"}},"elements":[]}}}]}`, upTarget)
	panel, line := convertedPanel(t, doc)
	d := displayOf(t, panel)
	if d.BackgroundColor != "#5794F2" || d.BackgroundImage == nil || d.BackgroundSize != "contain" || d.BorderColor != "#F2495C" || *d.BorderWidth != 2 || *d.BorderRadius != 6 {
		t.Errorf("display = %+v", d)
	}
	if line.Status != statusImported {
		t.Errorf("paths = %v", line.Paths)
	}
}

func TestCanvasLinksAreImported(t *testing.T) {
	el, line := oneCanvasElement(t, `{"type":"rectangle","name":"r",`+canvasScaled+`,"links":[`+
		`{"title":"Docs","url":"https://example.com","targetBlank":true,"oneClick":true},`+
		`{"title":"Bad","url":"javascript:alert(1)"},`+
		`{"title":"Local","url":"/d/abc"}]}`)
	want := []spec.Link{{Title: "Docs", URL: "https://example.com", NewTab: true}, {Title: "Local", URL: "/d/abc"}}
	if !slices.Equal(el.Links, want) || el.OneClick != "link" {
		t.Errorf("links = %+v oneClick %q", el.Links, el.OneClick)
	}
	if !slices.Equal(line.Paths, []string{"options.root.elements[0].links[1]"}) {
		t.Errorf("paths = %v", line.Paths)
	}
}

func fetchAction(fetch string) string {
	return `{"type":"fetch","title":"Restart","fetch":` + fetch + `}`
}

func actionElement(action string) string {
	return `{"type":"button","name":"b",` + canvasScaled + `,"config":{"text":{"mode":"fixed","fixed":"Go"}},"actions":[` + action + `]}`
}

func TestCanvasActionRoundTripsThroughTheSpec(t *testing.T) {
	panel, line := canvasDocPanel(t,
		`"templating":{"list":[{"type":"textbox","name":"env","query":"prod","current":{"text":"prod","value":"prod"}}]},`, "",
		actionElement(`{"type":"fetch","title":"Restart","confirmation":"Restart now?","oneClick":true,"fetch":{"method":"PUT","url":"https://example.com/run/${env}","body":"{\"a\":${__value.raw}}",`+
			`"headers":[["X-Env","${env}"],["Content-Type","text/plain"]],"queryParams":[["from","${__from}"]]}}`))
	els := canvasElements(t, panel)
	if len(els) != 1 || len(els[0].Actions) != 1 {
		t.Fatalf("elements = %+v paths %v reasons %v", els, line.Paths, line.Reasons)
	}
	got := els[0].Actions[0]
	want := spec.Action{
		Title: "Restart", Method: "PUT", URL: "https://example.com/run/${env}", ContentType: "text/plain", Body: `{"a":${__value.raw}}`,
		Confirm: "Restart now?", Headers: []spec.ActionParam{{Name: "X-Env", Value: "${env}"}}, Query: []spec.ActionParam{{Name: "from", Value: "${__from}"}},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) || els[0].OneClick != "action" {
		t.Errorf("action = %+v, want %+v, oneClick %q", got, want, els[0].OneClick)
	}
	data, err := json.Marshal(panel)
	if err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`{"specVersion":%q,"title":"T","time":{"from":"now-1h","to":"now"},"variables":[{"name":"env","type":"textbox"}],"panels":[%s]}`, spec.Version, data)
	if _, err := spec.Parse([]byte(doc)); err != nil {
		t.Fatalf("the imported action does not parse: %v\n%s", err, doc)
	}
	if line.Status != statusImported {
		t.Errorf("paths = %v, reasons = %v", line.Paths, line.Reasons)
	}
}

func TestCanvasActionKeepsTheActionWithoutACredentialHeader(t *testing.T) {
	el, line := oneCanvasElement(t, actionElement(fetchAction(`{"method":"POST","url":"https://example.com/restart","headers":[["Authorization","Bearer abc"],["X-Trace","1"]],"queryParams":[["api_key","k"],["a","b"]]}`)))
	if len(el.Actions) != 1 {
		t.Fatalf("actions = %+v", el.Actions)
	}
	action := el.Actions[0]
	if len(action.Headers) != 1 || action.Headers[0].Name != "X-Trace" || len(action.Query) != 1 || action.Query[0].Name != "a" {
		t.Errorf("action = %+v", action)
	}
	data, _ := json.Marshal(el)
	if strings.Contains(string(data), "Bearer") || strings.Contains(string(data), "api_key") {
		t.Errorf("a credential survived: %s", data)
	}
	if !slices.Contains(line.Reasons, canvasActionAuthDroppedReason) || line.Status != statusPartial {
		t.Errorf("reasons = %v", line.Reasons)
	}
}

func TestCanvasActionsThatCannotBeKeptAreLeftOutAndReported(t *testing.T) {
	cases := []struct{ name, fetch, reason string }{
		{"http address", `{"method":"POST","url":"http://example.com/restart"}`, canvasActionURLReason},
		{"variable host", `{"method":"POST","url":"https://${host}/restart"}`, canvasActionURLReason},
		{"relative address", `{"method":"POST","url":"/api/restart"}`, canvasActionURLReason},
		{"undeclared variable in the body", `{"method":"POST","url":"https://example.com/restart","body":"${nope}"}`, canvasActionReference},
		{"unknown method", `{"method":"TRACE","url":"https://example.com/restart"}`, canvasActionMethodReason},
		{"body over the limit", `{"method":"POST","url":"https://example.com/restart","body":"` + strings.Repeat("a", 10001) + `"}`, canvasActionBodyReason},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el, line := oneCanvasElement(t, actionElement(fetchAction(tc.fetch)))
			if len(el.Actions) != 0 || el.OneClick != "" {
				t.Errorf("actions = %+v", el.Actions)
			}
			if !slices.Contains(line.Reasons, tc.reason) || !slices.Contains(line.Paths, "options.root.elements[0].actions[0]") {
				t.Errorf("paths = %v, reasons = %v, want %q", line.Paths, line.Reasons, tc.reason)
			}
		})
	}
}

func TestCanvasInfinityActionsAreReported(t *testing.T) {
	el, line := oneCanvasElement(t, actionElement(`{"type":"infinity","title":"Run","infinity":{"method":"GET","url":"/run","datasourceUid":"inf"}}`))
	if len(el.Actions) != 0 || !slices.Contains(line.Reasons, canvasActionTypeReason) {
		t.Errorf("actions = %+v reasons = %v", el.Actions, line.Reasons)
	}
}

func TestCanvasButtonApiBecomesAnAction(t *testing.T) {
	el, line := oneCanvasElement(t, `{"type":"button","name":"b",`+canvasScaled+`,"config":{"text":{"mode":"fixed","fixed":"Deploy"},"api":{"endpoint":"https://example.com/hook","method":"POST","data":"{\"a\":1}","contentType":"application/json","queryParams":[["a","b"]],"headerParams":[["X-Key","1"]]}}}`)
	if len(el.Actions) != 1 {
		t.Fatalf("actions = %+v paths %v", el.Actions, line.Paths)
	}
	action := el.Actions[0]
	if action.Title != "Deploy" || action.URL != "https://example.com/hook" || action.Body != `{"a":1}` || len(action.Query) != 1 || len(action.Headers) != 1 {
		t.Errorf("action = %+v", action)
	}
	if line.Status != statusImported {
		t.Errorf("paths = %v", line.Paths)
	}
}

func TestCanvasConnectionOptions(t *testing.T) {
	elements := `{"type":"rectangle","name":"A",` + canvasScaled + `,"connections":[` +
		`{"targetName":"B","path":"curved","lineStyle":{"style":"dotted"},"direction":{"mode":"fixed","fixed":"both"},"size":{"field":"rate","min":2,"max":8},"vertices":[{"x":0.5,"y":0.5}]}]},` +
		`{"type":"rectangle","name":"B",` + canvasScaled + `}`
	panel, line := canvasPanel(t, 12, 10, elements)
	got := displayOf(t, panel).Connections
	want := []spec.CanvasConnection{{From: "A", To: "B", Path: "curved", LineStyle: "dotted", Arrow: "both", WidthField: "rate", MinWidth: 2, MaxWidth: 8}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("connections = %+v, want %+v", got, want)
	}
	if !slices.Equal(line.Paths, []string{"options.root.elements[0].connections[0].vertices"}) {
		t.Errorf("paths = %v", line.Paths)
	}
}

func TestCanvasElementKeepsOnlyTenLinksAndActions(t *testing.T) {
	var links, actions []string
	for range 11 {
		links = append(links, `{"title":"L","url":"https://example.com/"}`)
		actions = append(actions, fetchAction(`{"url":"https://example.com/run"}`))
	}
	el, _ := oneCanvasElement(t, `{"type":"rectangle","name":"r",`+canvasScaled+`,"links":[`+strings.Join(links, ",")+`],"actions":[`+strings.Join(actions, ",")+`]}`)
	if len(el.Links) != canvasLinkMax || len(el.Actions) != canvasActionMax {
		t.Errorf("links %d actions %d", len(el.Links), len(el.Actions))
	}
}
