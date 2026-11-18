package api_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/api"
)

func TestLoadParsesTheDocumentOnceAndKeepsItValid(t *testing.T) {
	first, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if first.Paths.Len() == 0 {
		t.Fatal("the document has no paths")
	}
	second, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("Load parsed the document again")
	}
}
