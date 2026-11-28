package proxyhttp

import (
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func marshalExemplars(q queryproxy.QueryResult) ([]byte, error) {
	return queryproxy.MarshalFrames([]queryproxy.QueryResult{{Ref: q.Ref, Frames: q.Exemplars}})
}

func joinFrames(parts [][]byte) []byte {
	size, nonEmpty := len("[]"), 0
	for _, part := range parts {
		if len(part) > len("[]") {
			size += len(part) - len("[]")
			nonEmpty++
		}
	}
	out := make([]byte, 1, size+max(nonEmpty-1, 0))
	out[0] = '['
	for _, part := range parts {
		if len(part) <= len("[]") {
			continue
		}
		inner := part[1 : len(part)-1]
		if len(out) > 1 {
			out = append(out, ',')
		}
		out = append(out, inner...)
	}
	return append(out, ']')
}
