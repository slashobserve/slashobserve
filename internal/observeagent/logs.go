package observeagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
)

const (
	maxLineBytes = 4096
	component    = "observe-agent"
	plainMessage = "observe-agent output"
)

var reservedFields = []string{"level", "msg", "component", "truncated"}

func relay(r io.Reader, logger *slog.Logger) {
	br := bufio.NewReaderSize(r, maxLineBytes+2)
	for {
		line, truncated, err := readLine(br)
		if line != "" || truncated {
			emit(logger, line, truncated)
		}
		if err != nil {
			return
		}
	}
}

func readLine(br *bufio.Reader) (line string, truncated bool, err error) {
	chunk, err := br.ReadSlice('\n')
	line = strings.TrimRight(string(chunk), "\r\n")
	if errors.Is(err, bufio.ErrBufferFull) {
		truncated = true
		for errors.Is(err, bufio.ErrBufferFull) {
			_, err = br.ReadSlice('\n')
		}
	}
	if len(line) > maxLineBytes {
		line = line[:maxLineBytes]
		truncated = true
	}
	return strings.ToValidUTF8(line, "�"), truncated, err
}

func emit(logger *slog.Logger, line string, truncated bool) {
	level, msg, attrs, ok := parseJSONLine(line)
	if !ok {
		level, msg, attrs = slog.LevelInfo, plainMessage, []any{slog.String("line", line)}
	}
	attrs = append(attrs, slog.String("component", component))
	if truncated {
		attrs = append(attrs, slog.Bool("truncated", true))
	}
	logger.Log(context.Background(), level, msg, attrs...)
}

func parseJSONLine(line string) (level slog.Level, msg string, attrs []any, ok bool) {
	if !strings.HasPrefix(line, "{") {
		return 0, "", nil, false
	}
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var fields map[string]any
	if err := dec.Decode(&fields); err != nil || dec.More() {
		return 0, "", nil, false
	}
	levelText, levelOK := fields["level"].(string)
	msg, msgOK := fields["msg"].(string)
	if !levelOK || !msgOK {
		return 0, "", nil, false
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !slices.Contains(reservedFields, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		if attr, ok := fieldAttr(k, fields[k]); ok {
			attrs = append(attrs, attr)
		}
	}
	return parseLevel(levelText), msg, attrs, true
}

func fieldAttr(key string, value any) (slog.Attr, bool) {
	switch v := value.(type) {
	case string:
		return slog.String(key, v), true
	case bool:
		return slog.Bool(key, v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return slog.Int64(key, i), true
		}
		if f, err := v.Float64(); err == nil {
			return slog.Float64(key, f), true
		}
		return slog.String(key, v.String()), true
	}
	return slog.Attr{}, false
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}
