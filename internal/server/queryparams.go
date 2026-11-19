package server

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
)

const (
	unknownQueryHeader = "X-Unknown-Query-Parameters"
	selectionPrefix    = "var-"
	maxReportedNames   = 10
)

var errUndocumentedQuery = errors.New("a query parameter the document does not list")

var reportableName = regexp.MustCompile(`^[A-Za-z0-9_.\[\]-]{1,64}$`)

type queryParams map[string]map[string]struct{}

var documentedQueryParams = sync.OnceValues(func() (queryParams, error) {
	doc, err := api.Load(context.Background())
	if err != nil {
		return nil, err
	}
	params := queryParams{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			names := map[string]struct{}{}
			for _, list := range []openapi3.Parameters{item.Parameters, op.Parameters} {
				for _, ref := range list {
					if ref.Value != nil && ref.Value.In == "query" {
						names[ref.Value.Name] = struct{}{}
					}
				}
			}
			params[method+" "+apiPrefix+path] = names
		}
	}
	return params, nil
})

func (q queryParams) unknown(method, template string, query map[string][]string) []string {
	known, ok := q[strings.ToUpper(method)+" "+template]
	if !ok {
		return nil
	}
	var unknown []string
	for name := range query {
		if _, documented := known[name]; !documented && !strings.HasPrefix(name, selectionPrefix) {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	return unknown
}

func warnUnknownQuery(params queryParams) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "HEAD" {
			method = "GET"
		}
		if unknown := params.unknown(method, ginToTemplate(c.FullPath()), c.Request.URL.Query()); len(unknown) > 0 {
			c.Header(unknownQueryHeader, reportedNames(unknown))
		}
		c.Next()
	}
}

func reportedNames(names []string) string {
	var shown []string
	for _, name := range names {
		if len(shown) == maxReportedNames {
			break
		}
		if reportableName.MatchString(name) {
			shown = append(shown, name)
		}
	}
	if len(shown) < len(names) {
		shown = append(shown, "...")
	}
	return strings.Join(shown, ", ")
}
