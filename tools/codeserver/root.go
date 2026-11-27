package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
)

type refusal string

func (r refusal) Error() string { return string(r) }

const (
	errOutside  refusal = "path is outside the root"
	errAbsolute refusal = "path must be relative to the root"
	errNotFound refusal = "not found"
	errDenied   refusal = "not readable"
	errInvalid  refusal = "path is not valid"
	errNotDir   refusal = "not a directory"
	errIsDir    refusal = "is a directory; use list_dir"
	errNotFile  refusal = "not a regular file"

	osEscapeText = "path escapes from parent"
)

type Root struct {
	os  *os.Root
	dir string
}

func NewRoot(dir string) (*Root, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	opened, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}
	return &Root{os: opened, dir: abs}, nil
}

func (r *Root) Close() error { return r.os.Close() }

func (r *Root) clean(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", errInvalid
	}
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return "", errAbsolute
	}
	cleaned := path.Clean(rel)
	if slices.Contains(strings.Split(cleaned, "/"), ".git") {
		return "", errNotFound
	}
	return cleaned, nil
}

func (r *Root) Stat(rel string) (fs.FileInfo, error) {
	name, err := r.clean(rel)
	if err != nil {
		return nil, err
	}
	info, err := r.os.Stat(name)
	if err != nil {
		return nil, fsRefusal(err)
	}
	return info, nil
}

func (r *Root) Lstat(rel string) (fs.FileInfo, error) {
	name, err := r.clean(rel)
	if err != nil {
		return nil, err
	}
	info, err := r.os.Lstat(name)
	if err != nil {
		return nil, fsRefusal(err)
	}
	return info, nil
}

func (r *Root) ReadDir(rel string) ([]fs.DirEntry, error) {
	return r.ReadDirN(rel, -1)
}

func (r *Root) ReadDirN(rel string, limit int) ([]fs.DirEntry, error) {
	name, err := r.clean(rel)
	if err != nil {
		return nil, err
	}
	info, err := r.os.Stat(name)
	if err != nil {
		return nil, fsRefusal(err)
	}
	if !info.IsDir() {
		return nil, errNotDir
	}
	f, err := r.os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fsRefusal(err)
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(limit)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fsRefusal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (r *Root) OpenRegular(rel string) (*os.File, fs.FileInfo, error) {
	name, err := r.clean(rel)
	if err != nil {
		return nil, nil, err
	}
	info, err := r.os.Stat(name)
	if err != nil {
		return nil, nil, fsRefusal(err)
	}
	if err := requireRegular(info); err != nil {
		return nil, nil, err
	}
	f, err := r.os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fsRefusal(err)
	}
	opened, err := f.Stat()
	if err == nil {
		err = requireRegular(opened)
	} else {
		err = fsRefusal(err)
	}
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, opened, nil
}

func requireRegular(info fs.FileInfo) error {
	switch {
	case info.IsDir():
		return errIsDir
	case !info.Mode().IsRegular():
		return errNotFile
	}
	return nil
}

func fsRefusal(err error) error {
	switch {
	case strings.Contains(err.Error(), osEscapeText):
		return errOutside
	case errors.Is(err, os.ErrNotExist):
		return errNotFound
	case errors.Is(err, os.ErrPermission):
		return errDenied
	case errors.Is(err, syscall.ENOTDIR):
		return errNotDir
	case errors.Is(err, syscall.ELOOP), errors.Is(err, syscall.ENAMETOOLONG):
		return errInvalid
	default:
		return err
	}
}
