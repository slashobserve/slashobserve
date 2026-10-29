package news

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	maxItems     = 50
	maxFeedTitle = 200
	maxItemTitle = 300
	maxSummary   = 500
	maxLinkBytes = 2000
	untitled     = "Untitled"
)

// xml.HTMLAutoClose lists link, which would close every RSS <link>text</link> empty.
var autoClose = slices.DeleteFunc(slices.Clone(xml.HTMLAutoClose), func(name string) bool { return name == "link" })

var ErrNotAFeed = errors.New("not an rss or atom feed")

type Feed struct {
	Title string
	Items []Item
}

type Item struct {
	Title     string
	Link      *string
	Published *time.Time
	Summary   string
}

type rssItem struct {
	Title       string   `xml:"title"`
	Links       []string `xml:"link"`
	GUID        rssGUID  `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Date        string   `xml:"date"`
	Description string   `xml:"description"`
	Encoded     string   `xml:"encoded"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rdfDocument struct {
	Channel rssChannel `xml:"channel"`
	Items   []rssItem  `xml:"item"`
}

type atomText struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
	Inner string `xml:",innerxml"`
}

func (t atomText) markup() string {
	if t.Type == "xhtml" {
		return t.Inner
	}
	return t.Value
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     atomText   `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Summary   atomText   `xml:"summary"`
	Content   atomText   `xml:"content"`
}

type atomDocument struct {
	Title   atomText    `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

func Parse(body []byte, headerCharset string, base *url.URL) (Feed, error) {
	dec := newDecoder(body, headerCharset)
	root, err := rootElement(dec)
	if err != nil {
		return Feed{}, err
	}
	switch root.Name.Local {
	case "rss":
		var doc rssDocument
		if err := dec.DecodeElement(&doc, &root); err != nil {
			return Feed{}, fmt.Errorf("decode rss: %w", err)
		}
		return fromRSS(doc.Channel.Title, doc.Channel.Items, base), nil
	case "RDF":
		var doc rdfDocument
		if err := dec.DecodeElement(&doc, &root); err != nil {
			return Feed{}, fmt.Errorf("decode rdf: %w", err)
		}
		return fromRSS(doc.Channel.Title, doc.Items, base), nil
	case "feed":
		var doc atomDocument
		if err := dec.DecodeElement(&doc, &root); err != nil {
			return Feed{}, fmt.Errorf("decode atom: %w", err)
		}
		return fromAtom(doc, base), nil
	}
	return Feed{}, ErrNotAFeed
}

func newDecoder(body []byte, headerCharset string) *xml.Decoder {
	var input io.Reader = bytes.NewReader(body)
	reader := charset.NewReaderLabel
	if label := strings.TrimSpace(headerCharset); label != "" && !isUTF8(label) {
		if converted, err := charset.NewReaderLabel(label, input); err == nil {
			input = converted
			reader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
		}
	}
	dec := xml.NewDecoder(input)
	dec.Strict = false
	dec.AutoClose = autoClose
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = reader
	return dec
}

func isUTF8(label string) bool {
	return strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "utf8")
}

func rootElement(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return xml.StartElement{}, ErrNotAFeed
		}
		if err != nil {
			return xml.StartElement{}, fmt.Errorf("read feed: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start, nil
		}
	}
}

func fromRSS(title string, items []rssItem, base *url.URL) Feed {
	feed := Feed{Title: plainText(title, maxFeedTitle), Items: []Item{}}
	for _, it := range items {
		if len(feed.Items) == maxItems {
			break
		}
		published := parseRSSDate(it.PubDate)
		if published == nil {
			published = parseRFC3339(it.Date)
		}
		summary := plainText(it.Description, maxSummary)
		if summary == "" {
			summary = plainText(it.Encoded, maxSummary)
		}
		if item, ok := newItem(plainText(it.Title, maxItemTitle), rssLink(it), base, published, summary); ok {
			feed.Items = append(feed.Items, item)
		}
	}
	return feed
}

func rssLink(it rssItem) string {
	for _, l := range it.Links {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	value := strings.TrimSpace(it.GUID.Value)
	if u, err := url.Parse(value); err == nil && u.IsAbs() && !strings.EqualFold(strings.TrimSpace(it.GUID.IsPermaLink), "false") {
		return value
	}
	return ""
}

func fromAtom(doc atomDocument, base *url.URL) Feed {
	feed := Feed{Title: plainText(doc.Title.markup(), maxFeedTitle), Items: []Item{}}
	for _, e := range doc.Entries {
		if len(feed.Items) == maxItems {
			break
		}
		published := parseRFC3339(e.Published)
		if published == nil {
			published = parseRFC3339(e.Updated)
		}
		summary := plainText(e.Summary.markup(), maxSummary)
		if summary == "" {
			summary = plainText(e.Content.markup(), maxSummary)
		}
		if item, ok := newItem(plainText(e.Title.markup(), maxItemTitle), atomAlternateLink(e), base, published, summary); ok {
			feed.Items = append(feed.Items, item)
		}
	}
	return feed
}

func atomAlternateLink(e atomEntry) string {
	for _, l := range e.Links {
		if (l.Rel == "" || l.Rel == "alternate") && strings.TrimSpace(l.Href) != "" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

func newItem(title, link string, base *url.URL, published *time.Time, summary string) (Item, bool) {
	if title == "" && summary == "" {
		return Item{}, false
	}
	if title == "" {
		title = untitled
	}
	return Item{Title: title, Link: safeLink(link, base), Published: published, Summary: summary}, true
}

func safeLink(raw string, base *url.URL) *string {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil
	}
	out := u.String()
	if len(out) > maxLinkBytes {
		return nil
	}
	return &out
}

func parseRFC3339(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

var zoneOffsets = map[string]string{
	"UT": "+0000", "GMT": "+0000", "UTC": "+0000", "Z": "+0000",
	"EST": "-0500", "EDT": "-0400", "CST": "-0600", "CDT": "-0500",
	"MST": "-0700", "MDT": "-0600", "PST": "-0800", "PDT": "-0700",
}

var rssLayouts = func() []string {
	var layouts []string
	for _, weekday := range []string{"Mon, ", ""} {
		for _, year := range []string{"2006", "06"} {
			for _, clock := range []string{"15:04:05", "15:04"} {
				layouts = append(layouts, weekday+"2 Jan "+year+" "+clock+" -0700")
			}
		}
	}
	return layouts
}()

func parseRSSDate(s string) *time.Time {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	if offset, ok := zoneOffsets[strings.ToUpper(fields[len(fields)-1])]; ok {
		fields[len(fields)-1] = offset
	}
	normalized := strings.Join(fields, " ")
	for _, layout := range rssLayouts {
		if t, err := time.Parse(layout, normalized); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
