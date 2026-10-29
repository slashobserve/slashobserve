package grafana

import (
	"net/url"
	"strings"
	"unicode"
)

const (
	grafanaBlogFeed = "https://grafana.com/blog/news.xml"
	maxFeedURLBytes = 2000
	newsHTTPOnly    = "Feeds are fetched over https only."
	newsFeedBad     = "This feed URL cannot be used."
	newsRepeat      = "Repeating news panels is not supported yet."
)

func (b *panelBuild) buildNews() *builtPanel {
	b.panel.Title = b.title()
	b.panel.Description = b.description()
	b.disp().FeedURL = b.newsFeedURL()
	b.consume("options.showImage", "options.useProxy")
	if asString(b.raw["repeat"]) != "" {
		b.partial("repeat", newsRepeat)
	}
	b.convertLinks()
	b.reportUnconsumed()
	return b.finish()
}

func (b *panelBuild) newsFeedURL() string {
	v, ok := b.take("options.feedUrl")
	if !ok || v == nil {
		return grafanaBlogFeed
	}
	raw, isString := v.(string)
	feed := strings.TrimSpace(raw)
	if isString && feed == "" {
		return grafanaBlogFeed
	}
	upgraded := false
	if scheme, rest, found := strings.Cut(feed, "://"); found && strings.EqualFold(scheme, "http") {
		feed, upgraded = "https://"+rest, true
	}
	if !isString || !usableFeedURL(feed) {
		b.partial("options.feedUrl", newsFeedBad)
		return ""
	}
	if upgraded {
		b.partial("options.feedUrl", newsHTTPOnly)
	}
	return feed
}

func usableFeedURL(feed string) bool {
	if len(feed) > maxFeedURLBytes || strings.IndexFunc(feed, unicode.IsSpace) >= 0 {
		return false
	}
	u, err := url.Parse(feed)
	return err == nil && u.Scheme == "https" && u.Host != ""
}
