package lifecycle

import "slices"

type DerivedState struct {
	Name string
	When string
}

type Derived struct {
	Name   string
	States []DerivedState
}

func (d Derived) Names() []string {
	out := make([]string, len(d.States))
	for i, s := range d.States {
		out[i] = s.Name
	}
	return out
}

func (d Derived) Has(name string) bool {
	return slices.Contains(d.Names(), name)
}

func (d Derived) Doc(title, intro string) Doc {
	doc := Doc{Title: title, Intro: intro, Derived: true}
	for _, s := range d.States {
		doc.States = append(doc.States, StateDoc{Name: s.Name, Meaning: s.When})
	}
	return doc
}
