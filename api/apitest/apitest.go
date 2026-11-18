package apitest

import (
	"context"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/slashobserve/slashobserve/api"
)

var (
	once sync.Once
	doc  *openapi3.T
	err  error
)

func Document(ctx context.Context) (*openapi3.T, error) {
	once.Do(func() { doc, err = api.Load(context.WithoutCancel(ctx)) })
	return doc, err
}

func Load(t testing.TB) *openapi3.T {
	t.Helper()
	d, err := Document(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return d
}
