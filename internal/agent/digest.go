package agent

import (
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const (
	maxDigestBytes = 4 << 10

	digestOpen   = "<discovered>\nNames this run already found; use them without listing them again.\n"
	digestClosed = "</discovered>"
)

type digest struct{ entries []tools.Discovery }

func (d *digest) add(x tools.Discovery) {
	if len(x.Names) == 0 {
		return
	}
	d.entries = slices.DeleteFunc(d.entries, func(e tools.Discovery) bool {
		return e.Datasource == x.Datasource && e.Of == x.Of
	})
	d.entries = append(d.entries, x)
}

func (d *digest) text() string {
	if len(d.entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(digestOpen)
	written := 0
	for _, e := range slices.Backward(d.entries) {
		line := "- " + e.Datasource + " " + e.Of + ": " + strings.Join(e.Names, ", ") + "\n"
		if b.Len()+len(line)+len(digestClosed) > maxDigestBytes {
			break
		}
		b.WriteString(line)
		written++
	}
	if written == 0 {
		return ""
	}
	b.WriteString(digestClosed)
	return b.String()
}
