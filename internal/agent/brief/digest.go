package brief

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

type Digest struct {
	Entries []tools.Discovery `json:"entries"`
}

func (d *Digest) Add(x tools.Discovery) {
	if len(x.Names) == 0 {
		return
	}
	d.Entries = slices.DeleteFunc(d.Entries, func(e tools.Discovery) bool {
		return e.Datasource == x.Datasource && e.Of == x.Of
	})
	d.Entries = append(d.Entries, x)
}

func (d *Digest) Text() string {
	if len(d.Entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(digestOpen)
	written := 0
	for _, e := range slices.Backward(d.Entries) {
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
