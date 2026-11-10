package activity

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidCursor = errors.New("cursor is not valid")

func encodeCursor(p Position) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d.%d", p.xid, p.seq)))
}

func decodeCursor(s string) (Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Position{}, ErrInvalidCursor
	}
	xidText, seqText, ok := strings.Cut(string(raw), ".")
	if !ok {
		return Position{}, ErrInvalidCursor
	}
	xid, err := strconv.ParseUint(xidText, 10, 64)
	if err != nil || xid == 0 {
		return Position{}, ErrInvalidCursor
	}
	seq, err := strconv.ParseInt(seqText, 10, 64)
	if err != nil || seq <= 0 {
		return Position{}, ErrInvalidCursor
	}
	return Position{xid: xid, seq: seq}, nil
}
