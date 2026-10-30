package grafana

import (
	"regexp"
	"strconv"
	"strings"
)

var shadePrefixes = []string{"super-light-", "semi-dark-", "light-", "dark-"}

var seriesPalette = map[string]string{
	"green": "#73BF69", "dark-green": "#37872D", "semi-dark-green": "#56A64B", "light-green": "#96D98D", "super-light-green": "#C8F2C2",
	"yellow": "#FADE2A", "dark-yellow": "#E0B400", "semi-dark-yellow": "#F2CC0C", "light-yellow": "#FFEE52", "super-light-yellow": "#FFF899",
	"red": "#F2495C", "dark-red": "#C4162A", "semi-dark-red": "#E02F44", "light-red": "#FF7383", "super-light-red": "#FFA6B0",
	"blue": "#5794F2", "dark-blue": "#1F60C4", "semi-dark-blue": "#3274D9", "light-blue": "#8AB8FF", "super-light-blue": "#C0D8FF",
	"orange": "#FF9830", "dark-orange": "#FA6400", "semi-dark-orange": "#FF780A", "light-orange": "#FFB357", "super-light-orange": "#FFCB7D",
	"purple": "#B877D9", "dark-purple": "#8F3BB8", "semi-dark-purple": "#A352CC", "light-purple": "#CA95E5", "super-light-purple": "#DEB6F2",
}

var continuousSchemes = map[string]string{
	"continuous-GrYlRd": "greenYellowRed", "continuous-RdYlGr": "redYellowGreen", "continuous-BlYlRd": "blueYellowRed",
	"continuous-YlRd": "yellowRed", "continuous-BlPu": "bluePurple", "continuous-YlBl": "yellowBlue",
	"continuous-blues": "blues", "continuous-reds": "reds", "continuous-greens": "greens", "continuous-purples": "purples",
	"continuous-viridis": "viridis", "continuous-magma": "magma", "continuous-plasma": "plasma",
	"continuous-inferno": "inferno", "continuous-cividis": "cividis",
}

var heatmapSchemes = map[string]string{
	"Blues": "blues", "Greens": "greens", "Oranges": "oranges", "Purples": "purples", "Reds": "reds",
	"RdYlGn": "redYellowGreen", "Spectral": "redYellowGreen", "RdYlBu": "blueYellowRed",
	"BuPu": "bluePurple", "YlOrRd": "yellowRed", "OrRd": "yellowRed", "YlGnBu": "yellowBlue",
	"Viridis": "viridis", "Magma": "magma", "Plasma": "plasma", "Inferno": "inferno", "Cividis": "cividis", "Warm": "warm",
}

var thresholdHexFamilies = map[string]string{
	"#73bf69": "ok", "#37872d": "ok", "#56a64b": "ok", "#96d98d": "ok", "#c8f2c2": "ok", "#299c46": "ok",
	"#f2495c": "critical", "#c4162a": "critical", "#e02f44": "critical", "#ff7383": "critical", "#ffa6b0": "critical", "#d44a3a": "critical",
	"#ff9830": "warning", "#fade2a": "warning", "#e0b400": "warning", "#f2cc0c": "warning", "#ffee52": "warning", "#fff899": "warning",
	"#fa6400": "warning", "#ff780a": "warning", "#ffb357": "warning", "#ffcb7d": "warning", "#eab839": "warning", "#ed8128": "warning",
	"#5794f2": "info", "#1f60c4": "info", "#3274d9": "info", "#8ab8ff": "info", "#c0d8ff": "info",
}

var thresholdNameFamilies = map[string]string{
	"green": "ok", "red": "critical", "orange": "warning", "yellow": "warning", "blue": "info",
}

var (
	hexColor  = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	rgbColor  = regexp.MustCompile(`^rgba?\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*(?:,\s*[0-9.]+\s*)?\)$`)
	hexLength = map[int]bool{4: true, 7: true, 9: true}
)

func baseColorName(name string) string {
	for _, prefix := range shadePrefixes {
		if rest, ok := strings.CutPrefix(name, prefix); ok {
			return rest
		}
	}
	return name
}

func hexColorValue(s string) (string, bool) {
	if !hexColor.MatchString(s) || !hexLength[len(s)] {
		return "", false
	}
	switch len(s) {
	case 4:
		return "#" + strings.Repeat(string(s[1]), 2) + strings.Repeat(string(s[2]), 2) + strings.Repeat(string(s[3]), 2), true
	case 9:
		return s[:7], true
	}
	return s, true
}

func rgbColorValue(s string) (string, bool) {
	m := rgbColor.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	out := "#"
	for _, part := range m[1:] {
		n, err := strconv.Atoi(part)
		if err != nil || n > 255 {
			return "", false
		}
		out += strings.ToLower(strconv.FormatInt(int64(n)+256, 16)[1:])
	}
	return out, true
}

func literalColor(s string) (string, bool) {
	if hex, ok := hexColorValue(s); ok {
		return hex, true
	}
	return rgbColorValue(s)
}

func seriesColor(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if hex, ok := seriesPalette[strings.ToLower(s)]; ok {
		return hex, true
	}
	return literalColor(s)
}

func thresholdColor(s string) (string, bool) {
	s = strings.TrimSpace(s)
	name := strings.ToLower(s)
	if family, ok := thresholdNameFamilies[baseColorName(name)]; ok && (name == baseColorName(name) || seriesPalette[name] != "") {
		return family, true
	}
	hex, ok := literalColor(s)
	if !ok {
		return seriesColor(s)
	}
	if family, ok := thresholdHexFamilies[strings.ToLower(hex)]; ok {
		return family, true
	}
	return hex, true
}
