package prompts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var annotationKeywords = map[string]bool{
	"$schema":     true,
	"$id":         true,
	"title":       true,
	"description": true,
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
	var out bytes.Buffer
	if err := writeSchema(&out, spec.Schema()); err != nil {
		panic("prompts: embedded schema does not strip: " + err.Error())
	}
	return out.Bytes()
})

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
			return writeSchemaArray(out, m.value)
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
	return writeMembers(out, members, func(m member) error { return writeSchema(out, m.value) }, nil)
}

func writeSchemaArray(out *bytes.Buffer, raw json.RawMessage) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	out.WriteByte('[')
	for i, item := range items {
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
