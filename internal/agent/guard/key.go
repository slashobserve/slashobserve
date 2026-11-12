package guard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func Key(name string, input json.RawMessage) string {
	var compact bytes.Buffer
	body := []byte(input)
	if json.Compact(&compact, input) == nil {
		body = compact.Bytes()
	}
	sum := sha256.Sum256(body)
	return name + ":" + hex.EncodeToString(sum[:])
}
