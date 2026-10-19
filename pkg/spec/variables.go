package spec

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	SelectAll  = "$__all"
	SelectAuto = "$__auto"
)

type VariableRegex struct {
	*regexp.Regexp
	Global bool
}

var firstNumber = regexp.MustCompile(`[0-9]+`)

func CompileVariableRegex(s string) (*VariableRegex, error) {
	pattern, global := "^"+s+"$", false
	if strings.HasPrefix(s, "/") {
		end := strings.LastIndex(s, "/")
		if end == 0 {
			return nil, fmt.Errorf("%q is not a valid regular expression: a /pattern/flags needs a closing slash", s)
		}
		prefix := ""
		for _, flag := range s[end+1:] {
			switch flag {
			case 'i', 'm', 's':
				if !strings.ContainsRune(prefix, flag) {
					prefix += string(flag)
				}
			case 'g':
				global = true
			case 'y':
			default:
				return nil, fmt.Errorf("%q is not a valid regular expression: unsupported flag %q", s, flag)
			}
		}
		pattern = s[1:end]
		if prefix != "" {
			pattern = "(?" + prefix + ")" + pattern
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%q is not a valid regular expression: %w", s, err)
	}
	return &VariableRegex{Regexp: re, Global: global}, nil
}

func ApplyVariableRegex(re *VariableRegex, values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	add := func(value string) {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	limit := 1
	if re.Global {
		limit = -1
	}
	for _, value := range values {
		matches := re.FindAllStringSubmatchIndex(value, limit)
		if len(matches) == 0 {
			continue
		}
		if picked, ok := namedGroup(re, value, matches); ok {
			add(picked)
			continue
		}
		if re.NumSubexp() == 0 {
			add(value)
			continue
		}
		for _, match := range matches {
			if picked, ok := capture(value, match, 1); ok {
				add(picked)
			}
		}
	}
	return out
}

func capture(value string, match []int, group int) (string, bool) {
	start, end := match[2*group], match[2*group+1]
	if start < 0 {
		return "", false
	}
	return value[start:end], true
}

func namedGroup(re *VariableRegex, value string, matches [][]int) (string, bool) {
	for _, name := range []string{"value", "text"} {
		at := re.SubexpIndex(name)
		if at < 0 {
			continue
		}
		for _, match := range matches {
			if picked, ok := capture(value, match, at); ok && picked != "" {
				return picked, true
			}
		}
	}
	return "", false
}

func SortVariableValues(values []string, order string) []string {
	out := slices.Clone(values)
	switch order {
	case "alpha":
		slices.SortStableFunc(out, compareUTF16)
	case "alphaDesc":
		slices.SortStableFunc(out, func(a, b string) int { return compareUTF16(b, a) })
	case "num", "numDesc":
		slices.SortStableFunc(out, func(a, b string) int {
			na, oka := leadingNumber(a)
			nb, okb := leadingNumber(b)
			switch {
			case oka && okb:
				return cmp.Compare(na, nb)
			case oka:
				return 1
			case okb:
				return -1
			}
			return 0
		})
		if order == "numDesc" {
			slices.Reverse(out)
		}
	}
	return out
}

func compareUTF16(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}

func leadingNumber(s string) (float64, bool) {
	found := firstNumber.FindString(s)
	if found == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(found, 64)
	return n, err == nil || errors.Is(err, strconv.ErrRange)
}
