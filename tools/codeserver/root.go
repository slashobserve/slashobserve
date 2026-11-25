package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type refusal string

func (r refusal) Error() string { return string(r) }

const (
	errOutside  refusal = "path is outside the root"
	errAbsolute refusal = "path must be relative to the root"
	errNotFound refusal = "not found"
	errDenied   refusal = "not readable"
	errInvalid  refusal = "path is not valid"
)

type Root struct {
	dir string
}

func NewRoot(dir string) (*Root, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("root is not a directory")
	}
	return &Root{dir: resolved}, nil
}

func (r *Root) Resolve(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", errInvalid
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", errAbsolute
	}
	joined := filepath.Join(r.dir, rel)
	if !within(r.dir, joined) {
		return "", errOutside
	}
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fsRefusal(err)
	}
	if !within(r.dir, resolved) {
		return "", errOutside
	}
	return resolved, nil
}

func (r *Root) Rel(path string) string {
	rel, err := filepath.Rel(r.dir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func fsRefusal(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return errNotFound
	case errors.Is(err, os.ErrPermission):
		return errDenied
	default:
		return err
	}
}
