package activity_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/activity"
)

const activityTypeRef = "#/components/schemas/ActivityType"

type typeArray struct {
	where    string
	maxItems *uint64
}

func collectTypeArrays(where string, ref *openapi3.SchemaRef, seen map[*openapi3.Schema]bool, out *[]typeArray) {
	if ref == nil || ref.Value == nil || seen[ref.Value] {
		return
	}
	seen[ref.Value] = true
	s := ref.Value
	if s.Items != nil && strings.HasSuffix(s.Items.Ref, activityTypeRef) {
		*out = append(*out, typeArray{where: where, maxItems: s.MaxItems})
	}
	collectTypeArrays(where+"[]", s.Items, seen, out)
	for name, prop := range s.Properties {
		collectTypeArrays(where+"."+name, prop, seen, out)
	}
	collectTypeArrays(where+"{}", s.AdditionalProperties.Schema, seen, out)
	for _, group := range [][]*openapi3.SchemaRef{s.AllOf, s.AnyOf, s.OneOf} {
		for _, member := range group {
			collectTypeArrays(where, member, seen, out)
		}
	}
}

func collectContent(where string, content openapi3.Content, seen map[*openapi3.Schema]bool, out *[]typeArray) {
	for mediaType, media := range content {
		collectTypeArrays(where+" "+mediaType, media.Schema, seen, out)
	}
}

func documentedTypeArrays(doc *openapi3.T) []typeArray {
	var out []typeArray
	seen := map[*openapi3.Schema]bool{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			name := fmt.Sprintf("%s (%s %s)", op.OperationID, method, path)
			for _, p := range slices.Concat(item.Parameters, op.Parameters) {
				collectTypeArrays(name+" parameter "+p.Value.Name, p.Value.Schema, seen, &out)
				collectContent(name+" parameter "+p.Value.Name, p.Value.Content, seen, &out)
			}
			if op.RequestBody != nil {
				collectContent(name+" request", op.RequestBody.Value.Content, seen, &out)
			}
			for status, response := range op.Responses.Map() {
				collectContent(name+" response "+status, response.Value.Content, seen, &out)
			}
		}
	}
	return out
}

func TestOpenAPIActivityTypesMatchTheLog(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	logged := activity.Types()

	schema := doc.Components.Schemas["ActivityType"]
	if schema == nil {
		t.Fatal("components.schemas.ActivityType is missing")
	}
	var documented []string
	for _, v := range schema.Value.Enum {
		documented = append(documented, fmt.Sprint(v))
	}
	var missing, extra []string
	for _, name := range logged {
		if !slices.Contains(documented, name) {
			missing = append(missing, name)
		}
	}
	for _, name := range documented {
		if !slices.Contains(logged, name) {
			extra = append(extra, name)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("the ActivityType enum is missing %v and has extra %v; fix api/openapi.yaml", missing, extra)
	}

	arrays := documentedTypeArrays(doc)
	if len(arrays) < 2 {
		t.Fatalf("found %d arrays of ActivityType, want at least the type filters of listActivity and streamActivity", len(arrays))
	}
	for _, a := range arrays {
		if a.maxItems == nil || *a.maxItems != uint64(len(logged)) {
			t.Errorf("%s allows %v activity types, but the log has %d; set maxItems to %d in api/paths", a.where, describeMax(a.maxItems), len(logged), len(logged))
		}
	}
	t.Logf("checked %d arrays of ActivityType against %d types", len(arrays), len(logged))
}

func describeMax(limit *uint64) string {
	if limit == nil {
		return "no limit set, so a client cannot tell how many"
	}
	return fmt.Sprint(*limit)
}
