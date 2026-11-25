package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	maxListEntries = 500
	maxReadLines   = 2000
	maxReadBytes   = 64 << 10
	maxReadFile    = 8 << 20
	maxLineBytes   = 4096
	maxGrepMatches = 100
	maxGrepBytes   = 24 << 10
	maxMatchText   = 300
	maxGrepFile    = 1 << 20
	sniffBytes     = 8000
	lineCutMarker  = " [line cut]"
)

type Code struct {
	root *Root
}

func (c *Code) ListDir(rel string) (string, error) {
	entries, err := c.root.ReadDir(rel)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	var out strings.Builder
	for i, e := range entries {
		if i == maxListEntries {
			fmt.Fprintf(&out, "[cut: %d of %d entries shown; use grep with a glob to find the rest]\n", maxListEntries, len(entries))
			break
		}
		out.WriteString(e.Name())
		if c.isDir(rel, e) {
			out.WriteByte('/')
		}
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func (c *Code) isDir(rel string, e fs.DirEntry) bool {
	if e.Type()&fs.ModeSymlink == 0 {
		return e.IsDir()
	}
	info, err := c.root.Stat(path.Join(rel, e.Name()))
	return err == nil && info.IsDir()
}

func (c *Code) ReadFile(ctx context.Context, rel string, offset, limit int) (string, error) {
	f, info, err := c.root.OpenRegular(rel)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if info.Size() > maxReadFile {
		return "", refusal(fmt.Sprintf("file is over %d bytes; find the part you need with grep", maxReadFile))
	}
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
	return readWindow(ctx, rd, offset, limit)
}

func isBinary(rd *bufio.Reader) bool {
	head, _ := rd.Peek(sniffBytes)
	return bytes.IndexByte(head, 0) >= 0
}

func readWindow(ctx context.Context, rd *bufio.Reader, offset, limit int) (string, error) {
	var out strings.Builder
	line := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		text, cut, err := readLine(rd, maxLineBytes)
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
		if cut {
			text += lineCutMarker
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

func readLine(rd *bufio.Reader, keep int) (string, bool, error) {
	var buf []byte
	cut := false
	for {
		part, more, err := rd.ReadLine()
		if err != nil {
			if len(buf) > 0 {
				break
			}
			return "", false, err
		}
		room := keep - len(buf)
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
	return strings.ToValidUTF8(string(buf), "�"), cut, nil
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
