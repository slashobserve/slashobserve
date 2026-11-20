package spec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var linkScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

func unsafeLinkScheme(url string) (string, bool) {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, strings.TrimFunc(url, func(r rune) bool { return r <= ' ' }))
	scheme := linkScheme.FindString(cleaned)
	if scheme == "" {
		return "", false
	}
	switch strings.ToLower(strings.TrimSuffix(scheme, ":")) {
	case "http", "https", "mailto":
		return "", false
	}
	return scheme, true
}

func checkLinkURL(url, path string) []Issue {
	scheme, unsafe := unsafeLinkScheme(url)
	if !unsafe {
		return nil
	}
	return []Issue{{
		Path:    path,
		Code:    CodeUnsafeLink,
		Message: fmt.Sprintf("a link may use http, https or mailto, or be a relative address; %q is not allowed", scheme),
	}}
}

func checkLinks(d *Dashboard, entries []PanelEntry) []Issue {
	var issues []Issue
	for i, l := range d.Links {
		issues = append(issues, checkLinkURL(l.URL, pointer("links", strconv.Itoa(i), "url"))...)
	}
	checkAll := func(links []Link, base string) {
		for i, l := range links {
			issues = append(issues, checkLinkURL(l.URL, base+pointer(strconv.Itoa(i), "url"))...)
		}
	}
	for _, e := range entries {
		checkAll(e.Panel.Links, e.Pointer+"/links")
		for k, o := range e.Panel.Overrides {
			checkAll(o.Links, e.Pointer+pointer("overrides", strconv.Itoa(k), "links"))
		}
		if e.Panel.Type != Canvas || e.Panel.Display == nil {
			continue
		}
		for k, el := range e.Panel.Display.Elements {
			checkAll(el.Links, e.Pointer+pointer("display", "elements", strconv.Itoa(k), "links"))
		}
	}
	return issues
}
