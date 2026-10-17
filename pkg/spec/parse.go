package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/slashobserve/slashobserve/spec/schema"
)

const (
	schemaID     = "https://slashobserve.com/schemas/observespec.v0.json"
	maxJSONDepth = 100
)

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.ObserveSpecV0))
	if err != nil {
		return nil, fmt.Errorf("parse embedded schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(schemaID, doc); err != nil {
		return nil, fmt.Errorf("add embedded schema: %w", err)
	}
	s, err := c.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema: %w", err)
	}
	return s, nil
})

func Parse(data []byte) (*Dashboard, error) {
	if len(data) > MaxBytes {
		return nil, newValidationError([]Issue{{
			Code:    CodeTooLarge,
			Message: fmt.Sprintf("document is %d bytes; the limit is %d bytes", len(data), MaxBytes),
		}})
	}
	if issues := scanJSON(data); len(issues) > 0 {
		return nil, newValidationError(issues)
	}
	issues, err := validateSchema(data)
	if err != nil {
		return nil, err
	}
	if len(issues) > 0 {
		return nil, newValidationError(issues)
	}
	var d Dashboard
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("spec: decode validated document: %w", err)
	}
	if issues := checkSemantics(&d); len(issues) > 0 {
		return nil, newValidationError(issues)
	}
	return &d, nil
}

type jsonScanner struct {
	dec    *json.Decoder
	issues []Issue
}

func scanJSON(data []byte) []Issue {
	s := &jsonScanner{dec: json.NewDecoder(bytes.NewReader(data))}
	s.dec.UseNumber()
	if err := s.value(nil, 0); err != nil {
		return []Issue{s.malformed(err)}
	}
	offset := s.dec.InputOffset()
	if _, err := s.dec.Token(); !errors.Is(err, io.EOF) {
		return []Issue{{
			Code:    CodeMalformedJSON,
			Message: fmt.Sprintf("unexpected data after the top-level JSON value at byte offset %d", offset),
		}}
	}
	return s.issues
}

func (s *jsonScanner) malformed(err error) Issue {
	var syn *json.SyntaxError
	offset := s.dec.InputOffset()
	switch {
	case errors.As(err, &syn):
		offset = syn.Offset
	case errors.Is(err, io.EOF):
		err = io.ErrUnexpectedEOF
	}
	return Issue{
		Code:    CodeMalformedJSON,
		Message: fmt.Sprintf("invalid JSON near byte offset %d: %v", offset, err),
	}
}

func (s *jsonScanner) value(path []string, depth int) error {
	tok, err := s.dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= maxJSONDepth {
		return fmt.Errorf("nesting is deeper than %d levels", maxJSONDepth)
	}
	if delim == '{' {
		return s.object(path, depth)
	}
	return s.array(path, depth)
}

func (s *jsonScanner) object(path []string, depth int) error {
	seen := map[string]struct{}{}
	for s.dec.More() {
		tok, err := s.dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		child := slices.Concat(path, []string{key})
		if _, dup := seen[key]; dup {
			s.issues = append(s.issues, Issue{
				Path:    pointer(child...),
				Code:    CodeDuplicateKey,
				Message: fmt.Sprintf("member %q appears more than once in the same object; keep a single value", key),
			})
		}
		seen[key] = struct{}{}
		if err := s.value(child, depth+1); err != nil {
			return err
		}
	}
	_, err := s.dec.Token()
	return err
}

func (s *jsonScanner) array(path []string, depth int) error {
	for i := 0; s.dec.More(); i++ {
		child := slices.Concat(path, []string{strconv.Itoa(i)})
		if err := s.value(child, depth+1); err != nil {
			return err
		}
	}
	_, err := s.dec.Token()
	return err
}

func validateSchema(data []byte) ([]Issue, error) {
	compiled, err := compiledSchema()
	if err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("spec: reparse validated document: %w", err)
	}
	err = compiled.Validate(instance)
	if err == nil {
		return numberIssues(instance, reflect.TypeFor[Dashboard](), nil), nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return nil, fmt.Errorf("spec: validate against schema: %w", err)
	}
	c := &issueCollector{printer: message.NewPrinter(language.English)}
	c.collect(ve)
	return c.issues, nil
}

type issueCollector struct {
	printer *message.Printer
	issues  []Issue
}

func (c *issueCollector) collect(ve *jsonschema.ValidationError) {
	causes := ve.Causes
	if _, ok := ve.ErrorKind.(*kind.OneOf); ok && len(causes) > 0 {
		kept, wants := pruneWrongBranches(causes)
		if len(kept) == 0 {
			c.issues = append(c.issues, Issue{
				Path:    pointer(ve.InstanceLocation...),
				Code:    CodeSchema,
				Message: fmt.Sprintf("\"type\" must be %s", strings.Join(wants, " or ")),
			})
			return
		}
		causes = kept
	}
	if len(causes) == 0 {
		c.issues = append(c.issues, c.leafIssue(ve))
		return
	}
	for _, cause := range causes {
		c.collect(cause)
	}
}

func (c *issueCollector) leafIssue(ve *jsonschema.ValidationError) Issue {
	issue := Issue{
		Path:    pointer(ve.InstanceLocation...),
		Code:    CodeSchema,
		Message: ve.ErrorKind.LocalizedString(c.printer),
	}
	if enum, ok := ve.ErrorKind.(*kind.Enum); ok && isUnitLocation(ve.InstanceLocation) {
		issue.Code = CodeUnknownUnit
		issue.Message = fmt.Sprintf("unknown unit %q; use a Grafana unit id such as \"reqps\", \"s\", \"bytes\" or \"percent\"", fmt.Sprint(enum.Got))
	}
	return issue
}

func isUnitLocation(loc []string) bool {
	return len(loc) == 3 && loc[0] == "panels" && loc[2] == "unit"
}

func pruneWrongBranches(causes []*jsonschema.ValidationError) (kept []*jsonschema.ValidationError, wants []string) {
	for _, cause := range causes {
		if w := typeConstWants(cause); len(w) > 0 {
			wants = append(wants, w...)
			continue
		}
		kept = append(kept, cause)
	}
	return kept, wants
}

func typeConstWants(ve *jsonschema.ValidationError) []string {
	if len(ve.Causes) == 0 {
		c, ok := ve.ErrorKind.(*kind.Const)
		if !ok || len(ve.InstanceLocation) == 0 || ve.InstanceLocation[len(ve.InstanceLocation)-1] != "type" {
			return nil
		}
		return []string{fmt.Sprintf("%q", fmt.Sprint(c.Want))}
	}
	var out []string
	for _, cause := range ve.Causes {
		out = append(out, typeConstWants(cause)...)
	}
	return out
}

func numberIssues(v any, t reflect.Type, path []string) []Issue {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch val := v.(type) {
	case map[string]any:
		if t.Kind() != reflect.Struct {
			return nil
		}
		return structNumberIssues(val, t, path)
	case []any:
		if t.Kind() != reflect.Slice {
			return nil
		}
		var out []Issue
		for i, item := range val {
			out = append(out, numberIssues(item, t.Elem(), slices.Concat(path, []string{strconv.Itoa(i)}))...)
		}
		return out
	case json.Number:
		return leafNumberIssues(val, t, path)
	}
	return nil
}

func structNumberIssues(obj map[string]any, t reflect.Type, path []string) []Issue {
	var out []Issue
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		item, ok := obj[name]
		if !ok {
			continue
		}
		out = append(out, numberIssues(item, f.Type, slices.Concat(path, []string{name}))...)
	}
	return out
}

func leafNumberIssues(n json.Number, t reflect.Type, path []string) []Issue {
	var msg string
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if _, err := strconv.ParseInt(n.String(), 10, t.Bits()); err != nil {
			msg = fmt.Sprintf("number %s must be written as a plain integer without a fraction or exponent", n)
		}
	case reflect.Float32, reflect.Float64:
		if _, err := strconv.ParseFloat(n.String(), t.Bits()); err != nil {
			msg = fmt.Sprintf("number %s is out of range", n)
		}
	}
	if msg == "" {
		return nil
	}
	return []Issue{{Path: pointer(path...), Code: CodeSchema, Message: msg}}
}
