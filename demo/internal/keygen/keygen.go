package keygen

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	keyPrefix = "sok_"
	keyMode   = 0o640
)

type Outcome string

const (
	Written Outcome = "written"
	Kept    Outcome = "kept"
)

func Run(path string) (Outcome, error) {
	if _, err := os.Stat(path); err == nil {
		return Kept, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("check %s: %w", path, err)
	}
	key, err := newKey()
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".keygen-*")
	if err != nil {
		return "", fmt.Errorf("create a key file next to %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(key + "\n"); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write a key file next to %s: %w", path, err)
	}
	if err := tmp.Chmod(keyMode); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("set the mode of a key file next to %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close a key file next to %s: %w", path, err)
	}
	if err := os.Link(tmp.Name(), path); errors.Is(err, fs.ErrExist) {
		return Kept, nil
	} else if err != nil {
		return "", fmt.Errorf("place the key file at %s: %w", path, err)
	}
	return Written, nil
}

func newKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate a key: %w", err)
	}
	return keyPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}
