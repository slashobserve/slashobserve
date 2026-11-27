package grounding

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

type nodeKind byte

const (
	nodeObject nodeKind = iota + 1
	nodeArray
	nodeString
	nodeNumber
	nodeLiteral
)

type node struct {
	kind nodeKind
	keys []string
	vals []*node
	text string
}

func stringNode(s string) *node { return &node{kind: nodeString, text: s} }

func parseJSON(raw []byte) (*node, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	n, err := decodeNode(dec)
	if err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return n, true
}

func decodeNode(dec *json.Decoder) (*node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		n := &node{kind: nodeArray}
		if t == '{' {
			n.kind = nodeObject
		}
		for dec.More() {
			if n.kind == nodeObject {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				n.keys = append(n.keys, key.(string))
			}
			child, err := decodeNode(dec)
			if err != nil {
				return nil, err
			}
			n.vals = append(n.vals, child)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return n, nil
	case string:
		return stringNode(t), nil
	case json.Number:
		return &node{kind: nodeNumber, text: string(t)}, nil
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		return nil, err
	}
	return &node{kind: nodeLiteral, text: string(raw)}, nil
}

func (n *node) get(key string) *node {
	if n == nil || n.kind != nodeObject {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

func (n *node) str(key string) string {
	if v := n.get(key); v != nil && v.kind == nodeString {
		return v.text
	}
	return ""
}

func (n *node) set(key string, v *node) {
	for i, k := range n.keys {
		if k == key {
			n.vals[i] = v
			return
		}
	}
	n.keys, n.vals = append(n.keys, key), append(n.vals, v)
}

func (n *node) encode() []byte {
	var b bytes.Buffer
	n.write(&b)
	return b.Bytes()
}

func (n *node) write(b *bytes.Buffer) {
	switch n.kind {
	case nodeObject:
		b.WriteByte('{')
		for i, key := range n.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, key)
			b.WriteByte(':')
			n.vals[i].write(b)
		}
		b.WriteByte('}')
	case nodeArray:
		b.WriteByte('[')
		for i, v := range n.vals {
			if i > 0 {
				b.WriteByte(',')
			}
			v.write(b)
		}
		b.WriteByte(']')
	case nodeString:
		writeString(b, n.text)
	default:
		b.WriteString(n.text)
	}
}

func writeString(b *bytes.Buffer, s string) {
	raw, _ := json.Marshal(s)
	b.Write(raw)
}
