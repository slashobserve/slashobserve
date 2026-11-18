package api

import (
	"context"
	"embed"
	"fmt"
	"net/url"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

//go:embed openapi.yaml paths/*.yaml
var files embed.FS

const rootName = "openapi.yaml"

var (
	loadMu sync.Mutex
	loaded *openapi3.T
)

func Load(ctx context.Context) (*openapi3.T, error) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loaded != nil {
		return loaded, nil
	}
	doc, err := load(ctx)
	if err != nil {
		return nil, err
	}
	loaded = doc
	return doc, nil
}

func load(ctx context.Context) (*openapi3.T, error) {
	data, err := files.ReadFile(rootName)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rootName, err)
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(_ *openapi3.Loader, location *url.URL) ([]byte, error) {
		return files.ReadFile(location.Path)
	}
	doc, err := loader.LoadFromDataWithPath(data, &url.URL{Path: rootName})
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", rootName, err)
	}
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate %s: %w", rootName, err)
	}
	return doc, nil
}
