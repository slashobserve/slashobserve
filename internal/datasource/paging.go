package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

const (
	MinPageSize    = 50
	MaxPageSize    = 500
	MaxCursorBytes = 2048

	cursorVersion       = 1
	scopeHexLen         = 16
	cursorForeign       = "cursor does not belong to this query"
	cursorUnreadable    = "cursor is not readable"
	PageTimestampShared = "more than page_size rows share one timestamp; raise page_size or narrow the query"
)

type TotalRelation string

const (
	TotalExact   TotalRelation = "eq"
	TotalAtLeast TotalRelation = "gte"
	TotalUnknown TotalRelation = "unknown"
)

type PageCursor struct {
	Scope  string   `json:"scope"`
	Page   int      `json:"page"`
	FromNs int64    `json:"from_ns"`
	ToNs   int64    `json:"to_ns"`
	EndNs  int64    `json:"end_ns"`
	Seen   []string `json:"seen,omitempty"`
}

type PageRequest struct {
	Query    Query
	Range    TimeRange
	Page     int
	PageSize int
	Cursor   *PageCursor
}

type PageInfo struct {
	Page, PageSize int
	Total          int64
	TotalRelation  TotalRelation
	HasNext        bool
	Next           *PageCursor
	Window         int
}

type PageSource interface {
	QueryPage(ctx context.Context, req PageRequest) ([]Frame, PageInfo, error)
	PagesByOffset() bool
}

type wireCursor struct {
	Version int `json:"v"`
	PageCursor
}

func CursorScope(uid, query string) string {
	return shortHash(uid + "\n" + query)
}

func EncodeCursor(c PageCursor) string {
	raw, err := json.Marshal(wireCursor{Version: cursorVersion, PageCursor: c})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeCursor(s, scope string) (*PageCursor, error) {
	if len(s) > MaxCursorBytes {
		return nil, Errorf(ErrInvalidQuery, "%s", cursorUnreadable)
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, Errorf(ErrInvalidQuery, "%s", cursorUnreadable)
	}
	var c wireCursor
	if err := json.Unmarshal(raw, &c); err != nil || c.Version != cursorVersion || c.Page < 1 {
		return nil, Errorf(ErrInvalidQuery, "%s", cursorUnreadable)
	}
	if c.Scope != scope {
		return nil, Errorf(ErrInvalidQuery, "%s", cursorForeign)
	}
	return &c.PageCursor, nil
}

func RowFingerprint(labels map[string]string, line string) string {
	pairs := make([]string, 0, len(labels))
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		pairs = append(pairs, k+"="+labels[k])
	}
	return shortHash(strings.Join(pairs, "\n") + "\n" + line)
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:scopeHexLen]
}

func PageTotal(page, pageSize, rows int, hasNext bool) (int64, TotalRelation) {
	total := int64(page-1)*int64(pageSize) + int64(rows)
	if hasNext {
		return total, TotalAtLeast
	}
	return total, TotalExact
}
