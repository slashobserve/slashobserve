package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"golang.org/x/term"
)

const noColorEnv = "NO_COLOR"

type output struct {
	out   io.Writer
	err   io.Writer
	json  bool
	quiet bool

	terminal bool
	color    palette
}

func (a *app) output() *output {
	terminal := isTerminal(a.std.Out)
	return &output{
		out: a.std.Out, err: a.std.Err, json: a.json, quiet: a.quiet,
		terminal: terminal,
		color:    palette{on: terminal && os.Getenv(noColorEnv) == ""},
	}
}

func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

const defaultTerminalWidth = 80

func terminalWidth(w io.Writer) int {
	if file, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(file.Fd())); err == nil && width > 0 {
			return width
		}
	}
	return defaultTerminalWidth
}

type palette struct{ on bool }

func (p palette) paint(code, s string) string {
	if !p.on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) green(s string) string { return p.paint("32", s) }

func (p palette) red(s string) string { return p.paint("31", s) }

func (p palette) yellow(s string) string { return p.paint("33", s) }

func (p palette) dim(s string) string { return p.paint("2", s) }

func (o *output) body(data []byte) {
	_, _ = o.out.Write(data)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		_, _ = fmt.Fprintln(o.out)
	}
}

func (o *output) line(format string, args ...any) {
	_, _ = fmt.Fprintf(o.out, format+"\n", args...)
}

func (o *output) notice(format string, args ...any) {
	if o.quiet {
		return
	}
	_, _ = fmt.Fprintf(o.err, format+"\n", args...)
}

func (o *output) warn(format string, args ...any) {
	_, _ = fmt.Fprintf(o.err, format+"\n", args...)
}

func (o *output) confirm(format string, args ...any) {
	if o.quiet {
		return
	}
	_, _ = fmt.Fprintf(o.out, format+"\n", args...)
}

func (o *output) table(headers []string, rows [][]string) {
	writer := tabwriter.NewWriter(o.out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, strings.Join(headers, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(writer, strings.Join(row, "\t"))
	}
	_ = writer.Flush()
}

func (o *output) fields(pairs [][2]string) {
	writer := tabwriter.NewWriter(o.out, 0, 0, 2, ' ', 0)
	for _, pair := range pairs {
		_, _ = fmt.Fprintf(writer, "%s\t%s\n", pair[0], pair[1])
	}
	_ = writer.Flush()
}

func text(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || hiddenFormatting(r) {
			return ' '
		}
		return r
	}, s)
}

func hiddenFormatting(r rune) bool {
	switch {
	case r >= 0x200b && r <= 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return r == 0x061c || r == 0x2028 || r == 0x2029 || r == 0xfeff
}

func when(raw string) string {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return text(raw)
	}
	return t.UTC().Format("2006-01-02T15:04Z")
}

func orDash(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return text(*s)
}
