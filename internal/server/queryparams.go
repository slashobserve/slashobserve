package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
)

const (
	unknownQueryHeader = "X-Unknown-Query-Parameters"
	selectionPrefix    = "var-"
	maxReportedNames   = 10

	listOperationPrefix = "list"
	invalidQueryCode    = "invalid_request"
)

var errUndocumentedQuery = errors.New("a query parameter the document does not list")

var reportableName = regexp.MustCompile(`^[A-Za-z0-9_.\[\]-]{1,64}$`)

type queryRoute struct {
	names      map[string]struct{}
	list       bool
	parameters []*openapi3.Parameter
}

type queryParams map[string]queryRoute

var documentedQueryParams = sync.OnceValues(func() (queryParams, error) {
	doc, err := api.Load(context.Background())
	if err != nil {
		return nil, err
	}
	params := queryParams{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			route := queryRoute{names: map[string]struct{}{}, list: method == http.MethodGet && strings.HasPrefix(op.OperationID, listOperationPrefix)}
			for _, list := range []openapi3.Parameters{item.Parameters, op.Parameters} {
				for _, ref := range list {
					if ref.Value != nil && ref.Value.In == "query" {
						route.names[ref.Value.Name] = struct{}{}
						route.parameters = append(route.parameters, ref.Value)
					}
				}
			}
			params[method+" "+apiPrefix+path] = route
		}
	}
	return params, nil
})

func (q queryParams) unknown(method, template string, query map[string][]string) []string {
	route, ok := q[strings.ToUpper(method)+" "+template]
	if !ok {
		return nil
	}
	var unknown []string
	for name := range query {
		if _, documented := route.names[name]; !documented && !strings.HasPrefix(name, selectionPrefix) {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	return unknown
}

func warnUnknownQuery(params queryParams) gin.HandlerFunc {
	defineFormats()
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		template := ginToTemplate(c.FullPath())
		query := c.Request.URL.Query()
		unknown := params.unknown(method, template, query)
		if route := params[strings.ToUpper(method)+" "+template]; route.list {
			if message := route.refusal(c.Request, unknown); message != "" {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": message, "code": invalidQueryCode})
				return
			}
		}
		if len(unknown) > 0 {
			c.Header(unknownQueryHeader, reportedNames(unknown))
		}
		c.Next()
	}
}

func (r queryRoute) refusal(req *http.Request, unknown []string) string {
	if len(unknown) > 0 {
		return fmt.Sprintf("unknown query parameter %s; this route takes %s", reportedNames(unknown), r.takes())
	}
	query, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return "the query string is malformed; escape ; and other reserved characters in a value"
	}
	input := &openapi3filter.RequestValidationInput{Request: req, Options: &openapi3filter.Options{SkipSettingDefaults: true}}
	for _, p := range r.parameters {
		if _, given := query[p.Name]; !given && !p.Required {
			continue
		}
		if openapi3filter.ValidateParameter(req.Context(), input, p) != nil {
			return fmt.Sprintf("query parameter %q must be %s", p.Name, expectation(p.Schema))
		}
	}
	return ""
}

func (r queryRoute) takes() string {
	names := make([]string, 0, len(r.names))
	for name := range r.names {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "no query parameters"
	}
	return strings.Join(names, ", ")
}

func expectation(ref *openapi3.SchemaRef) string {
	if ref == nil || ref.Value == nil {
		return "a valid value"
	}
	s := ref.Value
	switch {
	case len(s.Enum) > 0:
		values := make([]string, len(s.Enum))
		for i, v := range s.Enum {
			values[i] = fmt.Sprint(v)
		}
		return "one of " + strings.Join(values, ", ")
	case s.Type.Is("integer"):
		return "an integer" + bounds(s)
	case s.Type.Is("number"):
		return "a number" + bounds(s)
	case s.Type.Is("boolean"):
		return "true or false"
	case s.Type.Is("array"):
		return "a list in which each value is " + expectation(s.Items)
	case s.Format == "uuid":
		return "a UUID"
	case s.Format == "date-time":
		return "an RFC 3339 time"
	}
	return "a valid value" + lengths(s)
}

func bounds(s *openapi3.Schema) string {
	switch {
	case s.Min != nil && s.Max != nil:
		return " from " + strconv.FormatFloat(*s.Min, 'f', -1, 64) + " to " + strconv.FormatFloat(*s.Max, 'f', -1, 64)
	case s.Min != nil:
		return " of at least " + strconv.FormatFloat(*s.Min, 'f', -1, 64)
	case s.Max != nil:
		return " of at most " + strconv.FormatFloat(*s.Max, 'f', -1, 64)
	}
	return ""
}

func lengths(s *openapi3.Schema) string {
	switch {
	case s.MinLength > 0 && s.MaxLength != nil:
		return " of " + strconv.FormatUint(s.MinLength, 10) + " to " + strconv.FormatUint(*s.MaxLength, 10) + " characters"
	case s.MaxLength != nil:
		return " of at most " + strconv.FormatUint(*s.MaxLength, 10) + " characters"
	}
	return ""
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
