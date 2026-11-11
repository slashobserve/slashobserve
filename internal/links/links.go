package links

import (
	"net/url"
	"strings"
)

const (
	maxSlugLength = 60
	historyPath   = "versions"
)

func Slug(title string) string {
	var b strings.Builder
	pendingDash := false
	for i := 0; i < len(title); i++ {
		c := title[i]
		switch {
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		case 'A' <= c && c <= 'Z':
			c += 'a' - 'A'
		default:
			pendingDash = b.Len() > 0
			continue
		}
		if pendingDash {
			b.WriteByte('-')
			pendingDash = false
		}
		b.WriteByte(c)
	}
	slug := b.String()
	if len(slug) > maxSlugLength {
		slug = strings.TrimRight(slug[:maxSlugLength], "-")
	}
	if slug == historyPath {
		return ""
	}
	return slug
}

func Dashboard(base *url.URL, id, title string) string {
	if slug := Slug(title); slug != "" {
		return join(base, "d", id, slug)
	}
	return join(base, "d", id)
}

func Folder(base *url.URL, id string) string {
	return join(base, "f", id)
}

func Observation(base *url.URL, id string) string {
	return join(base, "o", id)
}

func Chat(base *url.URL, requestID string) string {
	return join(base, "c", requestID)
}

func Build(base *url.URL, requestID string) string {
	return join(base, "b", requestID)
}

func Finding(base *url.URL, id string) string {
	return join(base, "findings", id)
}

func Logs(base *url.URL, query url.Values) string {
	return withQuery(join(base, "logs"), query)
}

func Traces(base *url.URL, query url.Values) string {
	return withQuery(join(base, "traces"), query)
}

func Public(base *url.URL, token string) string {
	return join(base, "p", token)
}

func withQuery(address string, query url.Values) string {
	if len(query) == 0 {
		return address
	}
	return address + "?" + query.Encode()
}

func join(base *url.URL, segments ...string) string {
	if base == nil {
		escaped := make([]string, len(segments))
		for i, s := range segments {
			escaped[i] = url.PathEscape(s)
		}
		return "/" + strings.Join(escaped, "/")
	}
	return base.JoinPath(segments...).String()
}
