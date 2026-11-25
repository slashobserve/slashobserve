package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
)

const maxWalkDepth = 40

var errGrepFull = refusal("grep full")

type glob struct {
	segments []string
	byName   bool
}

func parseGlob(pattern string) (*glob, error) {
	if pattern == "" {
		return nil, nil
	}
	segments := strings.Split(pattern, "/")
	for _, s := range segments {
		if s == "**" {
			continue
		}
		if strings.Contains(s, "**") {
			return nil, refusal("glob: ** must be a whole path segment, as in **/*.go")
		}
		if _, err := path.Match(s, ""); err != nil {
			return nil, refusal("glob is not valid")
		}
	}
	return &glob{segments: segments, byName: len(segments) == 1}, nil
}

func (g *glob) matches(rel string) bool {
	if g == nil {
		return true
	}
	if g.byName {
		ok, _ := path.Match(g.segments[0], path.Base(rel))
		return ok
	}
	return matchSegments(g.segments, strings.Split(rel, "/"))
}

func matchSegments(pattern, parts []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for i := 0; i <= len(parts); i++ {
				if matchSegments(pattern[1:], parts[i:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], parts[0]); !ok {
			return false
		}
		pattern, parts = pattern[1:], parts[1:]
	}
	return len(parts) == 0
}

func (c *Code) Grep(ctx context.Context, pattern, rel, globPattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", refusal("pattern is not a valid regular expression")
	}
	g, err := parseGlob(globPattern)
	if err != nil {
		return "", err
	}
	start, err := c.root.Stat(rel)
	if err != nil {
		return "", err
	}
	run := &grepRun{root: c.root, re: re, glob: g}
	if start.IsDir() {
		err = run.walk(ctx, path.Clean(rel), start, nil)
	} else {
		err = run.file(ctx, path.Clean(rel))
	}
	if err != nil && !errors.Is(err, errGrepFull) {
		return "", err
	}
	return run.result(), nil
}

type grepRun struct {
	root    *Root
	re      *regexp.Regexp
	glob    *glob
	out     strings.Builder
	matches int
	skipped int
	cut     bool
}

func (g *grepRun) walk(ctx context.Context, rel string, info fs.FileInfo, ancestors []fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := g.root.ReadDir(rel)
	if err != nil {
		return nil
	}
	ancestors = append(ancestors, info)
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		child := path.Join(rel, e.Name())
		childInfo, err := e.Info()
		if err != nil {
			continue
		}
		if childInfo.Mode()&fs.ModeSymlink != 0 {
			if childInfo, err = g.root.Stat(child); err != nil {
				continue
			}
		}
		switch {
		case childInfo.IsDir():
			if len(ancestors) >= maxWalkDepth || isAncestor(ancestors, childInfo) {
				continue
			}
			err = g.walk(ctx, child, childInfo, ancestors)
		case childInfo.Mode().IsRegular():
			err = g.file(ctx, child)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func isAncestor(ancestors []fs.FileInfo, info fs.FileInfo) bool {
	for _, a := range ancestors {
		if os.SameFile(a, info) {
			return true
		}
	}
	return false
}

func (g *grepRun) file(ctx context.Context, rel string) error {
	if !g.glob.matches(rel) {
		return nil
	}
	f, info, err := g.root.OpenRegular(rel)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	if info.Size() > maxGrepFile {
		g.skipped++
		return nil
	}
	rd := bufio.NewReaderSize(f, 64<<10)
	if isBinary(rd) {
		return nil
	}
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		text, _, err := readLine(rd, maxGrepFile)
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
