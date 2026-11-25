package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxListEntries = 500
	maxReadLines   = 2000
	maxReadBytes   = 64 << 10
	maxLineBytes   = 4096
	maxGrepMatches = 100
	maxGrepBytes   = 24 << 10
	maxMatchText   = 300
	maxGrepFile    = 1 << 20
	sniffBytes     = 8000
)

type Code struct {
	root *Root
}

func (c *Code) ListDir(rel string) (string, error) {
	dir, err := c.root.Resolve(rel)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fsRefusal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out strings.Builder
	for i, e := range entries {
		if i == maxListEntries {
			fmt.Fprintf(&out, "[cut: %d of %d entries shown; use grep with a glob to find the rest]\n", maxListEntries, len(entries))
			break
		}
		out.WriteString(e.Name())
		if e.IsDir() {
			out.WriteByte('/')
		}
		out.WriteByte('\n')
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	return out.String(), nil
}

func (c *Code) ReadFile(rel string, offset, limit int) (string, error) {
	f, err := c.openRegular(rel)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if offset < 1 {
		offset = 1
	}
	if limit < 1 || limit > maxReadLines {
		limit = maxReadLines
	}
	rd := bufio.NewReaderSize(f, 64<<10)
	if isBinary(rd) {
		return "", refusal("binary file")
	}
	return readWindow(rd, offset, limit)
}

func (c *Code) openRegular(rel string) (*os.File, error) {
	file, err := c.root.Resolve(rel)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(file) //nolint:gosec // file was resolved inside the root by Root.Resolve
	if err != nil {
		return nil, fsRefusal(err)
	}
	info, err := f.Stat()
	switch {
	case err != nil:
		err = fsRefusal(err)
	case info.IsDir():
		err = refusal("is a directory; use list_dir")
	case !info.Mode().IsRegular():
		err = refusal("not a regular file")
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func isBinary(rd *bufio.Reader) bool {
	head, _ := rd.Peek(sniffBytes)
	return bytes.IndexByte(head, 0) >= 0
}

func readWindow(rd *bufio.Reader, offset, limit int) (string, error) {
	var out strings.Builder
	line := 0
	for {
		text, err := readLine(rd)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fsRefusal(err)
		}
		line++
		if line < offset {
			continue
		}
		if line >= offset+limit {
			fmt.Fprintf(&out, "[cut: line limit reached; continue with offset=%d]\n", line)
			break
		}
		if out.Len()+len(text) > maxReadBytes {
			fmt.Fprintf(&out, "[cut: size limit reached; continue with offset=%d]\n", line)
			break
		}
		fmt.Fprintf(&out, "%6d\t%s\n", line, text)
	}
	if out.Len() > 0 {
		return out.String(), nil
	}
	if line == 0 {
		return "(empty file)", nil
	}
	return fmt.Sprintf("(no lines at offset %d; the file has %d)", offset, line), nil
}

func (c *Code) Grep(ctx context.Context, pattern, rel, glob string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", refusal("pattern is not a valid regular expression")
	}
	if glob != "" {
		if _, err := path.Match(glob, ""); err != nil {
			return "", refusal("glob is not valid")
		}
	}
	start, err := c.root.Resolve(rel)
	if err != nil {
		return "", err
	}
	g := &grepRun{code: c, re: re, glob: glob}
	err = filepath.WalkDir(start, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" && p != start {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return g.file(p)
	})
	if err != nil && !errors.Is(err, errGrepFull) {
		return "", err
	}
	return g.result(), nil
}

var errGrepFull = refusal("grep full")

type grepRun struct {
	code    *Code
	re      *regexp.Regexp
	glob    string
	out     strings.Builder
	matches int
	skipped int
	cut     bool
}

func (g *grepRun) file(p string) error {
	rel := g.code.root.Rel(p)
	if !globMatches(g.glob, rel) {
		return nil
	}
	f, err := os.Open(p) //nolint:gosec // p comes from walking a directory resolved inside the root
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	if info.Size() > maxGrepFile {
		g.skipped++
		return nil
	}
	rd := bufio.NewReaderSize(f, 64<<10)
	if isBinary(rd) {
		return nil
	}
	for n := 1; ; n++ {
		text, err := readLine(rd)
		if err != nil {
			return nil
		}
		if !g.re.MatchString(text) {
			continue
		}
		entry := fmt.Sprintf("%s:%d: %s\n", rel, n, clip(text, maxMatchText))
		if g.matches == maxGrepMatches || g.out.Len()+len(entry) > maxGrepBytes {
			g.cut = true
			return errGrepFull
		}
		g.out.WriteString(entry)
		g.matches++
	}
}

func (g *grepRun) result() string {
	var notes []string
	if g.cut {
		notes = append(notes, fmt.Sprintf("[cut: stopped after %d matches; narrow the pattern, path or glob]", g.matches))
	}
	if g.skipped > 0 {
		notes = append(notes, fmt.Sprintf("[%d files over %d bytes were not searched]", g.skipped, maxGrepFile))
	}
	if g.matches == 0 {
		notes = append([]string{"no matches"}, notes...)
	}
	return g.out.String() + strings.Join(notes, "\n")
}

func globMatches(glob, rel string) bool {
	if glob == "" {
		return true
	}
	if strings.Contains(glob, "/") {
		ok, _ := path.Match(glob, rel)
		return ok
	}
	ok, _ := path.Match(glob, path.Base(rel))
	return ok
}

func readLine(rd *bufio.Reader) (string, error) {
	var buf []byte
	cut := false
	for {
		part, more, err := rd.ReadLine()
		if err != nil {
			if len(buf) > 0 {
				break
			}
			return "", err
		}
		room := maxLineBytes - len(buf)
		switch {
		case room <= 0:
			cut = cut || len(part) > 0
		case len(part) > room:
			buf = append(buf, part[:room]...)
			cut = true
		default:
			buf = append(buf, part...)
		}
		if !more {
			break
		}
	}
	text := strings.ToValidUTF8(string(buf), "�")
	if cut {
		text += " [line cut]"
	}
	return text, nil
}

func clip(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
