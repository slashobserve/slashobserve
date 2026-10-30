package prompts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const omitFromPromptKeyword = "x-omitFromPrompt"

var annotationKeywords = map[string]bool{
	"$schema":     true,
	"$id":         true,
	"title":       true,
	"description": true,

	omitFromPromptKeyword: true,
}

var schemaMapKeywords = map[string]bool{
	"properties": true,
	"$defs":      true,
}

var schemaArrayKeywords = map[string]bool{
	"allOf":       true,
	"anyOf":       true,
	"oneOf":       true,
	"prefixItems": true,
}

var schemaKeywords = map[string]bool{
	"items":                 true,
	"additionalProperties":  true,
	"unevaluatedProperties": true,
	"if":                    true,
	"then":                  true,
	"else":                  true,
	"not":                   true,
}

var schema = sync.OnceValue(func() []byte {
	out, err := stripSchema(spec.Schema())
	if err != nil {
		panic("prompts: embedded schema does not strip: " + err.Error())
	}
	return out
})

func stripSchema(raw json.RawMessage) ([]byte, error) {
	var out bytes.Buffer
	if err := writeSchema(&out, raw); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func omittedFromPrompt(raw json.RawMessage) bool {
	members, isObject, err := objectMembers(raw)
	if err != nil || !isObject {
		return false
	}
	for _, m := range members {
		if m.key == omitFromPromptKeyword {
			return string(bytes.TrimSpace(m.value)) == "true"
		}
	}
	return false
}

func Schema() []byte { return bytes.Clone(schema()) }

type member struct {
	key   string
	value json.RawMessage
}

func objectMembers(raw json.RawMessage) ([]member, bool, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil, false, err
	}
	var members []member
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, false, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, false, fmt.Errorf("object key is %T", keyToken)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false, err
		}
		members = append(members, member{key: key, value: value})
	}
	return members, true, nil
}

func writeSchema(out *bytes.Buffer, raw json.RawMessage) error {
	members, isObject, err := objectMembers(raw)
	if err != nil {
		return err
	}
	if !isObject {
		return json.Compact(out, raw)
	}
	return writeMembers(out, members, func(m member) error {
		switch {
		case schemaMapKeywords[m.key]:
			return writeSchemaMap(out, m.value)
		case schemaArrayKeywords[m.key]:
			return writeSchemaArray(out, m.key, m.value)
		case schemaKeywords[m.key]:
			return writeSchema(out, m.value)
		default:
			return json.Compact(out, m.value)
		}
	}, annotationKeywords)
}

func writeSchemaMap(out *bytes.Buffer, raw json.RawMessage) error {
	members, isObject, err := objectMembers(raw)
	if err != nil {
		return err
	}
	if !isObject {
		return json.Compact(out, raw)
	}
	kept := make([]member, 0, len(members))
	for _, m := range members {
		if !omittedFromPrompt(m.value) {
			kept = append(kept, m)
		}
	}
	return writeMembers(out, kept, func(m member) error { return writeSchema(out, m.value) }, nil)
}

func writeSchemaArray(out *bytes.Buffer, keyword string, raw json.RawMessage) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	omittable := keyword == "anyOf" || keyword == "oneOf"
	kept := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if !omittedFromPrompt(item) {
			kept = append(kept, item)
			continue
		}
		if !omittable {
			return fmt.Errorf("%s inside %s", omitFromPromptKeyword, keyword)
		}
	}
	if omittable && len(kept) == 0 {
		return fmt.Errorf("%s leaves %s empty", omitFromPromptKeyword, keyword)
	}
	out.WriteByte('[')
	for i, item := range kept {
		if i > 0 {
			out.WriteByte(',')
		}
		if err := writeSchema(out, item); err != nil {
			return err
		}
	}
	out.WriteByte(']')
	return nil
}

func writeMembers(out *bytes.Buffer, members []member, writeValue func(member) error, skip map[string]bool) error {
	out.WriteByte('{')
	first := true
	for _, m := range members {
		if skip[m.key] {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		key, err := json.Marshal(m.key)
		if err != nil {
			return err
		}
		out.Write(key)
		out.WriteByte(':')
		if err := writeValue(m); err != nil {
			return err
		}
	}
	out.WriteByte('}')
	return nil
}
