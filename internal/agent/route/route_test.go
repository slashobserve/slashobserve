package route

import (
	"encoding/json"
	"os"
	"testing"
)

type routeCases struct {
	Types    []string `json:"types"`
	Classify []struct {
		Text string `json:"text"`
		Want Task   `json:"want"`
	} `json:"classify"`
	Shape []struct {
		Name     string `json:"name"`
		Question struct {
			Investigation bool     `json:"investigation"`
			Dashboard     bool     `json:"dashboard"`
			Panel         bool     `json:"panel"`
			Observation   bool     `json:"observation"`
			Types         []string `json:"types"`
			Text          string   `json:"text"`
		} `json:"question"`
		Want Task `json:"want"`
	} `json:"shape"`
}

func readCases(t *testing.T) routeCases {
	t.Helper()
	root, err := os.OpenRoot("../../../spec/examples")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile("agent/route/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases routeCases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Classify) == 0 || len(cases.Shape) == 0 {
		t.Fatal("the route cases are empty")
	}
	return cases
}

func TestClassifyPicksOneTaskTypeOrFallsBack(t *testing.T) {
	cases := readCases(t)
	for _, c := range cases.Classify {
		if _, ok := Parse(string(c.Want)); !ok {
			t.Fatalf("%q wants the unknown task %q", c.Text, c.Want)
		}
		if got := Classify(Question{Types: cases.Types, Text: c.Text}); got != c.Want {
			t.Errorf("%q: got %s, want %s", c.Text, got.Name(), c.Want.Name())
		}
	}
}

func TestClassifyReadsTheRunsShape(t *testing.T) {
	for _, c := range readCases(t).Shape {
		q := c.Question
		got := Classify(Question{Investigation: q.Investigation, Dashboard: q.Dashboard, Panel: q.Panel, Observation: q.Observation, Types: q.Types, Text: q.Text})
		if got != c.Want {
			t.Errorf("%s: got %s, want %s", c.Name, got.Name(), c.Want.Name())
		}
	}
}

func TestParseKnowsEveryTask(t *testing.T) {
	for _, task := range append([]Task{Full}, Tasks...) {
		if got, ok := Parse(string(task)); !ok || got != task {
			t.Errorf("Parse(%q) = %q, %v", task, got, ok)
		}
	}
	if _, ok := Parse("dashboard"); ok {
		t.Error("Parse accepts an unknown task")
	}
}
