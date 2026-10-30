package spec

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

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
	if issues := byteIssues(data, 0); len(issues) > 0 {
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

func byteIssues(data []byte, depth int) []Issue {
	if !utf8.Valid(data) {
		return []Issue{{
			Code:    CodeMalformedJSON,
			Message: "document is not valid UTF-8",
		}}
	}
	if offset := loneSurrogate(data); offset >= 0 {
		return []Issue{{
			Code:    CodeMalformedJSON,
			Message: fmt.Sprintf("document has an unpaired UTF-16 surrogate escape at byte offset %d", offset),
		}}
	}
	return scanJSON(data, depth)
}

func loneSurrogate(data []byte) int {
	inString := false
	for i := 0; i < len(data); i++ {
		switch {
		case data[i] == '"':
			inString = !inString
		case !inString || data[i] != '\\':
		case i+1 < len(data) && data[i+1] == 'u':
			r, ok := hexEscape(data, i)
			if !ok {
				continue
			}
			switch {
			case r >= 0xDC00 && r <= 0xDFFF:
				return i
			case r >= 0xD800 && r <= 0xDBFF:
				low, ok := hexEscape(data, i+6)
				if !ok || low < 0xDC00 || low > 0xDFFF {
					return i
				}
				i += 11
			default:
				i += 5
			}
		default:
			i++
		}
	}
	return -1
}

func hexEscape(data []byte, at int) (rune, bool) {
	if at+6 > len(data) || data[at] != '\\' || data[at+1] != 'u' {
		return 0, false
	}
	b, err := hex.DecodeString(string(data[at+2 : at+6]))
	if err != nil {
		return 0, false
	}
	return rune(b[0])<<8 | rune(b[1]), true
}

type jsonScanner struct {
	dec    *json.Decoder
	issues []Issue
}

func scanJSON(data []byte, depth int) []Issue {
	s := &jsonScanner{dec: json.NewDecoder(bytes.NewReader(data))}
	s.dec.UseNumber()
	if err := s.value(nil, depth); err != nil {
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
	return c.finish(), nil
}

type issueCollector struct {
	printer     *message.Printer
	issues      []Issue
	unevaluated []string
}

func isUnevaluated(ve *jsonschema.ValidationError) bool {
	_, ok := ve.ErrorKind.(*kind.FalseSchema)
	return ok && strings.HasSuffix(ve.SchemaURL, "/unevaluatedProperties")
}

func (c *issueCollector) finish() []Issue {
	byParent := map[string][]string{}
	var parents []string
	for _, loc := range c.unevaluated {
		if slices.ContainsFunc(c.issues, func(i Issue) bool { return i.Path == loc || strings.HasPrefix(i.Path, loc+"/") }) {
			continue
		}
		cut := strings.LastIndex(loc, "/")
		parent, name := loc[:cut], pointerUnescaper.Replace(loc[cut+1:])
		if !slices.Contains(byParent[parent], name) {
			if _, ok := byParent[parent]; !ok {
				parents = append(parents, parent)
			}
			byParent[parent] = append(byParent[parent], name)
		}
	}
	issues := c.issues
	for _, parent := range parents {
		issues = append(issues, Issue{
			Path:    parent,
			Code:    CodeSchema,
			Message: fmt.Sprintf("additional properties %s not allowed", quoteNames(byParent[parent])),
		})
	}
	return dedupeIssues(issues)
}

func dedupeIssues(issues []Issue) []Issue {
	type key struct{ path, message string }
	seen := make(map[key]struct{}, len(issues))
	out := issues[:0]
	for _, issue := range issues {
		k := key{issue.Path, issue.Message}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, issue)
	}
	return out
}

func (c *issueCollector) collect(ve *jsonschema.ValidationError) {
	if _, ok := ve.ErrorKind.(*kind.AnyOf); ok && isUnitLocation(ve.InstanceLocation) {
		c.issues = append(c.issues, unitIssue(ve))
		return
	}
	causes := ve.Causes
	if _, ok := ve.ErrorKind.(*kind.OneOf); ok && len(causes) > 0 {
		if branch, ok := presentExclusiveBranch(ve); ok {
			c.collect(branch)
			return
		}
		if message, ok := exclusiveMembersMessage(ve); ok {
			c.issues = append(c.issues, Issue{Path: pointer(ve.InstanceLocation...), Code: CodeSchema, Message: message})
			return
		}
		if missing, ok := commonMissing(ve.InstanceLocation, causes); ok {
			c.issues = append(c.issues, Issue{
				Path:    pointer(ve.InstanceLocation...),
				Code:    CodeSchema,
				Message: missingMessage(missing),
			})
			for _, message := range c.commonOwnErrors(ve.InstanceLocation, causes) {
				c.issues = append(c.issues, Issue{Path: pointer(ve.InstanceLocation...), Code: CodeSchema, Message: message})
			}
			c.collectNested(ve.InstanceLocation, causes)
			return
		}
		kept, wants := pruneWrongBranches(ve.InstanceLocation, causes)
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
		if isUnevaluated(ve) {
			c.unevaluated = append(c.unevaluated, pointer(ve.InstanceLocation...))
			return
		}
		c.issues = append(c.issues, c.leafIssue(ve))
		return
	}
	for _, cause := range causes {
		c.collect(cause)
	}
}

type exclusiveMembers struct {
	message string
	keys    []string
}

var exclusiveMembersByRule = map[string]exclusiveMembers{
	schemaID + "#/$defs/match": {
		message: "match must have exactly one of query, series, seriesRegex, field, fieldType, value",
		keys:    []string{"query", "series", "seriesRegex", "field", "fieldType", "value"},
	},
	schemaID + "#/$defs/dataPanel/properties/queries/items": {
		message: "a query must have exactly one of query, expression, source",
		keys:    []string{"query", "expression", "source"},
	},
	schemaID + "#/$defs/mapping": {
		message: "mapping must have exactly one of value, range, regex, special",
		keys:    []string{"value", "range", "regex", "special"},
	},
}

func presentExclusiveBranch(ve *jsonschema.ValidationError) (*jsonschema.ValidationError, bool) {
	rule, ok := exclusiveMembersByRule[ve.SchemaURL]
	if !ok || len(ve.Causes) != len(rule.keys) {
		return nil, false
	}
	var present *jsonschema.ValidationError
	for i, cause := range ve.Causes {
		if missesKey(ve.InstanceLocation, cause, rule.keys[i]) {
			continue
		}
		if present != nil {
			return nil, false
		}
		present = cause
	}
	return present, present != nil
}

func missesKey(loc []string, ve *jsonschema.ValidationError, key string) bool {
	if r, ok := ve.ErrorKind.(*kind.Required); ok {
		return slices.Equal(ve.InstanceLocation, loc) && slices.Contains(r.Missing, key)
	}
	return slices.ContainsFunc(ve.Causes, func(cause *jsonschema.ValidationError) bool {
		return missesKey(loc, cause, key)
	})
}

func exclusiveMembersMessage(ve *jsonschema.ValidationError) (string, bool) {
	rule, ok := exclusiveMembersByRule[ve.SchemaURL]
	if !ok {
		return "", false
	}
	for _, cause := range ve.Causes {
		if !shapeErrorsOnly(ve.InstanceLocation, cause) {
			return "", false
		}
	}
	return rule.message, true
}

func shapeErrorsOnly(loc []string, ve *jsonschema.ValidationError) bool {
	if !slices.Equal(ve.InstanceLocation, loc) {
		return false
	}
	if len(ve.Causes) == 0 {
		switch ve.ErrorKind.(type) {
		case *kind.Required, *kind.AdditionalProperties:
			return true
		}
		return false
	}
	return !slices.ContainsFunc(ve.Causes, func(cause *jsonschema.ValidationError) bool {
		return !shapeErrorsOnly(loc, cause)
	})
}

func (c *issueCollector) leafIssue(ve *jsonschema.ValidationError) Issue {
	issue := Issue{
		Path:    pointer(ve.InstanceLocation...),
		Code:    CodeSchema,
		Message: ve.ErrorKind.LocalizedString(c.printer),
	}
	if _, ok := ve.ErrorKind.(*kind.FalseSchema); ok {
		issue.Message = "this property is not allowed with the other properties written here"
	}
	return issue
}

func unitIssue(ve *jsonschema.ValidationError) Issue {
	return Issue{
		Path: pointer(ve.InstanceLocation...),
		Code: CodeUnknownUnit,
		Message: fmt.Sprintf(
			"unknown unit %q; use a Grafana unit id such as \"reqps\", \"s\", \"bytes\" or \"percent\", or a custom \"suffix:<text>\" or \"prefix:<text>\"",
			unitValue(ve),
		),
	}
}

func unitValue(ve *jsonschema.ValidationError) string {
	if enum, ok := ve.ErrorKind.(*kind.Enum); ok {
		return fmt.Sprint(enum.Got)
	}
	for _, cause := range ve.Causes {
		if got := unitValue(cause); got != "" {
			return got
		}
	}
	return ""
}

func isUnitLocation(loc []string) bool {
	return len(loc) >= 3 && loc[len(loc)-1] == "unit" && (loc[len(loc)-3] == "panels" || loc[len(loc)-3] == "overrides")
}

func pruneWrongBranches(loc []string, causes []*jsonschema.ValidationError) (kept []*jsonschema.ValidationError, wants []string) {
	typeLocation := append(slices.Clone(loc), "type")
	for _, cause := range causes {
		if w := typeConstWants(typeLocation, cause); len(w) > 0 {
			wants = append(wants, w...)
			continue
		}
		kept = append(kept, cause)
	}
	return kept, wants
}

func quoteNames(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "'" + name + "'"
	}
	return strings.Join(quoted, ", ")
}

func missingMessage(missing []string) string {
	if len(missing) == 1 {
		return "missing property " + quoteNames(missing)
	}
	return "missing properties " + quoteNames(missing)
}

func commonMissing(loc []string, causes []*jsonschema.ValidationError) ([]string, bool) {
	var common []string
	for i, cause := range causes {
		missing, ok := requiredAt(loc, cause)
		if !ok {
			return nil, false
		}
		if i == 0 {
			common = missing
			continue
		}
		common = slices.DeleteFunc(common, func(name string) bool { return !slices.Contains(missing, name) })
	}
	return common, len(common) > 0
}

func requiredAt(loc []string, ve *jsonschema.ValidationError) ([]string, bool) {
	if !slices.Equal(ve.InstanceLocation, loc) {
		return nil, false
	}
	if r, ok := ve.ErrorKind.(*kind.Required); ok {
		return slices.Clone(r.Missing), slices.Contains(r.Missing, "type")
	}
	for _, cause := range ve.Causes {
		if missing, ok := requiredAt(loc, cause); ok {
			return missing, true
		}
	}
	return nil, false
}

func (c *issueCollector) commonOwnErrors(loc []string, causes []*jsonschema.ValidationError) []string {
	var common, commonExtras []string
	for i, cause := range causes {
		own, extras := c.ownErrors(loc, cause)
		if i == 0 {
			common, commonExtras = own, extras
			continue
		}
		common = slices.DeleteFunc(common, func(message string) bool { return !slices.Contains(own, message) })
		commonExtras = slices.DeleteFunc(commonExtras, func(name string) bool { return !slices.Contains(extras, name) })
	}
	if len(commonExtras) > 0 {
		slices.Sort(commonExtras)
		common = append(common, fmt.Sprintf("additional properties %s not allowed", quoteNames(commonExtras)))
	}
	return common
}

func (c *issueCollector) ownErrors(loc []string, ve *jsonschema.ValidationError) (messages, extras []string) {
	if !slices.Equal(ve.InstanceLocation, loc) {
		return nil, nil
	}
	if len(ve.Causes) == 0 {
		switch k := ve.ErrorKind.(type) {
		case *kind.Required:
			return nil, nil
		case *kind.AdditionalProperties:
			return nil, k.Properties
		}
		return []string{ve.ErrorKind.LocalizedString(c.printer)}, nil
	}
	for _, cause := range ve.Causes {
		m, e := c.ownErrors(loc, cause)
		messages, extras = append(messages, m...), append(extras, e...)
	}
	return messages, extras
}

func (c *issueCollector) collectNested(loc []string, causes []*jsonschema.ValidationError) {
	nested := &issueCollector{printer: c.printer}
	for _, cause := range causes {
		nested.collectBelow(loc, cause)
	}
	for _, issue := range nested.issues {
		if !slices.Contains(c.issues, issue) {
			c.issues = append(c.issues, issue)
		}
	}
	for _, loc := range nested.unevaluated {
		if !slices.Contains(c.unevaluated, loc) {
			c.unevaluated = append(c.unevaluated, loc)
		}
	}
}

func (c *issueCollector) collectBelow(loc []string, ve *jsonschema.ValidationError) {
	if len(ve.InstanceLocation) > len(loc) {
		c.collect(ve)
		return
	}
	for _, cause := range ve.Causes {
		c.collectBelow(loc, cause)
	}
}

func typeConstWants(typeLocation []string, ve *jsonschema.ValidationError) []string {
	if len(ve.Causes) == 0 {
		if !slices.Equal(ve.InstanceLocation, typeLocation) {
			return nil
		}
		switch k := ve.ErrorKind.(type) {
		case *kind.Const:
			return []string{fmt.Sprintf("%q", fmt.Sprint(k.Want))}
		case *kind.Enum:
			out := make([]string, len(k.Want))
			for i, want := range k.Want {
				out[i] = fmt.Sprintf("%q", fmt.Sprint(want))
			}
			return out
		}
		return nil
	}
	var out []string
	for _, cause := range ve.Causes {
		out = append(out, typeConstWants(typeLocation, cause)...)
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
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
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
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			out = append(out, structNumberIssues(obj, f.Type, path)...)
			continue
		}
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
