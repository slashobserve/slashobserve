package apitest_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/api/apitest"
)

func TestLoadParsesTheDocumentOnceAndKeepsItValid(t *testing.T) {
	first := apitest.Load(t)
	if first.Paths.Len() == 0 {
		t.Fatal("the document has no paths")
	}
	if second := apitest.Load(t); first != second {
		t.Fatal("Load parsed the document again")
	}
}
