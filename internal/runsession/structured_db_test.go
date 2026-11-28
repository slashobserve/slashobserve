package runsession

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
)

type listed []datasource.Instance

func (l listed) Visible(context.Context) []datasource.Instance { return l }

func withDatasources(d Datasources) rigOption {
	return func(c *Config) { c.Datasources = d }
}

func TestBriefCarriesTheStructuredBriefFromTheSameLoads(t *testing.T) {
	r := newRig(t, withDatasources(listed{
		{UID: "prom", Audience: datasource.AudienceEveryone},
		{UID: "admins", Audience: datasource.AudienceAdmins},
		{UID: auditsource.UID, Audience: datasource.AudienceEveryone},
	}))
	req := r.revise()
	r.startBuild(req, "a")
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	b := got.Brief
	if b == nil {
		t.Fatal("no structured brief")
	}
	if b.Request.Mode != "revise" || b.Request.Prompt != "add a latency panel" || b.Request.Attempt != req.Attempt || b.Request.PanelID != nil {
		t.Fatalf("request = %+v", b.Request)
	}
	if b.Current == nil || b.Current.Number != 1 || b.Current.Spec == nil || b.Current.Spec.Title != "t" {
		t.Fatalf("current = %+v", b.Current)
	}
	if b.Build == nil || b.Build.Progress != *got.Build || b.Build.BaseVersion != 1 || b.Build.Spec == nil {
		t.Fatalf("build = %+v, progress %+v", b.Build, got.Build)
	}
	if len(b.Datasources) != 1 || b.Datasources[0].UID != "prom" || !b.Datasources[0].Everyone || !b.Datasources[0].Healthy {
		t.Fatalf("datasources = %+v", b.Datasources)
	}
	if b.Guide.Text == "" || !json.Valid(b.Guide.Schema) || b.Withheld || b.Draft != nil {
		t.Fatalf("guide %d bytes, schema valid %v, withheld %v, draft %s", len(b.Guide.Text), json.Valid(b.Guide.Schema), b.Withheld, b.Draft)
	}
	everyone := r.svc.everyoneReads(context.Background())
	if everyone("admins") || everyone(auditsource.UID) || !everyone("prom") {
		t.Fatal("the Admins-only and the audit datasource must not be marked as read by everyone")
	}
}

func TestTheStructuredBriefOfASuggestionHoldsItsDraftPanel(t *testing.T) {
	r := newRig(t)
	req := r.suggest()
	req.Draft = json.RawMessage(`{"id":"a","type":"stat","title":"Draft","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}`)
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Brief == nil || string(got.Brief.Draft) != string(req.Draft) || got.Brief.Request.PanelID == nil || *got.Brief.Request.PanelID != "a" {
		t.Fatalf("brief = %+v", got.Brief)
	}
	if got.Brief.Current == nil {
		t.Fatal("a suggestion's brief has its base version")
	}
	if p, ok := got.Brief.Current.Spec.FindPanel("a"); !ok || p.Panel.Title != "Draft" {
		t.Fatalf("current panel = %+v", p)
	}
}
