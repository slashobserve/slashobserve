package news

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func mustBase(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustParse(t *testing.T, body, headerCharset string) Feed {
	t.Helper()
	feed, err := Parse([]byte(body), headerCharset, mustBase(t, "https://example.com/feed.xml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return feed
}

func at(t *testing.T, raw string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestParseRSS(t *testing.T) {
	feed := mustParse(t, `<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
  <title>Example &amp; Co</title>
  <atom:link href="https://example.com/feed.xml" rel="self"/>
  <item>
    <title>First</title>
    <link>https://example.com/first</link>
    <pubDate>Tue, 28 Oct 2025 09:30:00 +0200</pubDate>
    <description><![CDATA[<p>Hi&nbsp;<b>there</b></p><script>x()</script>]]></description>
  </item>
  <item>
    <title>Second</title>
    <atom:link href="https://example.com/ignored" rel="alternate"/>
    <guid isPermaLink="true">https://example.com/second</guid>
    <pubDate>Mon, 6 Oct 2025 08:00:00 GMT</pubDate>
    <content:encoded><![CDATA[<div>Full body</div>]]></content:encoded>
  </item>
  <item>
    <title>Third</title>
    <guid isPermaLink="false">https://example.com/not-a-link</guid>
    <pubDate>Mon, 06 Oct 2025 08:00 EST</pubDate>
  </item>
</channel>
</rss>`, "")

	if feed.Title != "Example & Co" {
		t.Errorf("title = %q", feed.Title)
	}
	if len(feed.Items) != 3 {
		t.Fatalf("items = %d", len(feed.Items))
	}
	first := feed.Items[0]
	if first.Title != "First" || first.Summary != "Hi there" {
		t.Errorf("first = %+v", first)
	}
	if first.Link == nil || *first.Link != "https://example.com/first" {
		t.Errorf("first link = %v", first.Link)
	}
	if first.Published == nil || !first.Published.Equal(at(t, "2025-10-28T07:30:00Z")) || first.Published.Location() != time.UTC {
		t.Errorf("first published = %v", first.Published)
	}
	second := feed.Items[1]
	if second.Link == nil || *second.Link != "https://example.com/second" || second.Summary != "Full body" {
		t.Errorf("second = %+v link %v", second, second.Link)
	}
	if second.Published == nil || !second.Published.Equal(at(t, "2025-10-06T08:00:00Z")) {
		t.Errorf("second published = %v", second.Published)
	}
	third := feed.Items[2]
	if third.Link != nil {
		t.Errorf("third link = %v, want nil", *third.Link)
	}
	if third.Published == nil || !third.Published.Equal(at(t, "2025-10-06T13:00:00Z")) {
		t.Errorf("third published = %v", third.Published)
	}
}

func TestParseRDF(t *testing.T) {
	feed := mustParse(t, `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel><title>Old school</title></channel>
  <item>
    <title>Entry</title>
    <link>https://example.com/entry</link>
    <description>Plain description</description>
    <dc:date>2025-03-01T10:00:00+01:00</dc:date>
  </item>
</rdf:RDF>`, "")
	if feed.Title != "Old school" || len(feed.Items) != 1 {
		t.Fatalf("feed = %+v", feed)
	}
	item := feed.Items[0]
	if item.Title != "Entry" || item.Summary != "Plain description" || item.Link == nil || *item.Link != "https://example.com/entry" {
		t.Errorf("item = %+v", item)
	}
	if item.Published == nil || !item.Published.Equal(at(t, "2025-03-01T09:00:00Z")) {
		t.Errorf("published = %v", item.Published)
	}
}

func TestParseAtom(t *testing.T) {
	feed := mustParse(t, `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title type="html">Atom &lt;b&gt;feed&lt;/b&gt;</title>
  <entry>
    <title>With published</title>
    <link rel="self" href="https://example.com/self"/>
    <link rel="alternate" href="https://example.com/alt"/>
    <published>2025-05-01T12:00:00Z</published>
    <updated>2025-06-01T12:00:00Z</updated>
    <summary>Short summary</summary>
    <content type="html">&lt;p&gt;Long content&lt;/p&gt;</content>
  </entry>
  <entry>
    <title>Updated only</title>
    <link href="/relative"/>
    <updated>2025-06-01T12:00:00+02:00</updated>
    <content type="html">&lt;p&gt;Only content&lt;/p&gt;</content>
  </entry>
  <entry>
    <title>Inline markup</title>
    <summary type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml">Some <b>bold</b> words</div></summary>
  </entry>
</feed>`, "")
	if feed.Title != "Atom feed" || len(feed.Items) != 3 {
		t.Fatalf("feed = %+v", feed)
	}
	a, b, c := feed.Items[0], feed.Items[1], feed.Items[2]
	if a.Link == nil || *a.Link != "https://example.com/alt" || a.Summary != "Short summary" {
		t.Errorf("a = %+v link %v", a, a.Link)
	}
	if a.Published == nil || !a.Published.Equal(at(t, "2025-05-01T12:00:00Z")) {
		t.Errorf("a published = %v", a.Published)
	}
	if b.Link == nil || *b.Link != "https://example.com/relative" || b.Summary != "Only content" {
		t.Errorf("b = %+v link %v", b, b.Link)
	}
	if b.Published == nil || !b.Published.Equal(at(t, "2025-06-01T10:00:00Z")) {
		t.Errorf("b published = %v", b.Published)
	}
	if c.Summary != "Some bold words" || c.Link != nil || c.Published != nil {
		t.Errorf("c = %+v", c)
	}
}

func TestParseCharsets(t *testing.T) {
	const prolog = "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>"
	const headless = "<?xml version=\"1.0\"?>"
	body := "<rss><channel><title>Caf\xe9</title><item><title>Cr\xe8me br\xfbl\xe9e</title></item></channel></rss>"
	cases := []struct {
		name   string
		body   string
		header string
	}{
		{"declared in the prolog", prolog + body, ""},
		{"declared by the header only", headless + body, "windows-1252"},
		{"header wins over the prolog", strings.Replace(prolog, "ISO-8859-1", "UTF-8", 1) + body, "iso-8859-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feed := mustParse(t, tc.body, tc.header)
			if feed.Title != "Café" || len(feed.Items) != 1 || feed.Items[0].Title != "Crème brûlée" {
				t.Errorf("feed = %+v", feed)
			}
		})
	}
}

func TestParseLinks(t *testing.T) {
	cases := []struct {
		name string
		link string
		want string
	}{
		{"absolute https", "https://other.example/post", "https://other.example/post"},
		{"absolute http", "http://other.example/post", "http://other.example/post"},
		{"path relative to the feed", "/posts/1", "https://example.com/posts/1"},
		{"relative to the feed directory", "posts/1", "https://example.com/posts/1"},
		{"javascript", "javascript:alert(1)", ""},
		{"mailto", "mailto:someone@example.com", ""},
		{"data", "data:text/html,hi", ""},
		{"user info", "https://user:pw@x.example/", ""},
		{"no host", "https:///path", ""},
		{"too long", "https://x.example/" + strings.Repeat("a", 2000), ""},
		{"just under the limit", "https://x.example/" + strings.Repeat("a", 1900), "https://x.example/" + strings.Repeat("a", 1900)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feed := mustParse(t, fmt.Sprintf(`<rss><channel><item><title>T</title><link>%s</link></item></channel></rss>`, tc.link), "")
			got := feed.Items[0].Link
			switch {
			case tc.want == "" && got != nil:
				t.Errorf("link = %q, want nil", *got)
			case tc.want != "" && (got == nil || *got != tc.want):
				t.Errorf("link = %v, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRSSDates(t *testing.T) {
	cases := []struct {
		name string
		date string
		want string
	}{
		{"RFC 1123 with zone name", "Tue, 28 Oct 2025 09:30:00 GMT", "2025-10-28T09:30:00Z"},
		{"RFC 1123Z", "Tue, 28 Oct 2025 09:30:00 +0530", "2025-10-28T04:00:00Z"},
		{"single digit day", "Wed, 1 Oct 2025 09:30:00 +0000", "2025-10-01T09:30:00Z"},
		{"RFC 822 two digit year", "28 Oct 25 09:30 PST", "2025-10-28T17:30:00Z"},
		{"RFC 822Z without weekday", "28 Oct 25 09:30 +0100", "2025-10-28T08:30:00Z"},
		{"surrounding space", "  Tue, 28 Oct 2025 09:30:00 UT \n", "2025-10-28T09:30:00Z"},
		{"unparsable", "yesterday-ish", ""},
		{"unknown zone name", "Tue, 28 Oct 2025 09:30:00 CEST", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feed := mustParse(t, fmt.Sprintf(`<rss><channel><item><title>T</title><pubDate>%s</pubDate></item></channel></rss>`, tc.date), "")
			got := feed.Items[0].Published
			switch {
			case tc.want == "" && got != nil:
				t.Errorf("published = %v, want nil", got)
			case tc.want != "" && (got == nil || !got.Equal(at(t, tc.want))):
				t.Errorf("published = %v, want %s", got, tc.want)
			}
		})
	}
}

func TestParseItemRules(t *testing.T) {
	t.Run("caps at 50 items in feed order", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<rss><channel><title>T</title>")
		for i := range 60 {
			fmt.Fprintf(&b, "<item><title>Item %d</title></item>", i)
		}
		b.WriteString("</channel></rss>")
		feed := mustParse(t, b.String(), "")
		if len(feed.Items) != 50 || feed.Items[0].Title != "Item 0" || feed.Items[49].Title != "Item 49" {
			t.Errorf("items = %d, first %q, last %q", len(feed.Items), feed.Items[0].Title, feed.Items[len(feed.Items)-1].Title)
		}
	})
	t.Run("caps a long title with an ellipsis", func(t *testing.T) {
		feed := mustParse(t, "<rss><channel><item><title>"+strings.Repeat("é", 400)+"</title></item></channel></rss>", "")
		title := feed.Items[0].Title
		if utf8.RuneCountInString(title) != 300 || !strings.HasSuffix(title, "…") {
			t.Errorf("title has %d runes and ends %q", utf8.RuneCountInString(title), title[len(title)-4:])
		}
	})
	t.Run("an item without title or summary is skipped", func(t *testing.T) {
		feed := mustParse(t, "<rss><channel><item><link>https://example.com/x</link></item><item><title>Kept</title></item></channel></rss>", "")
		if len(feed.Items) != 1 || feed.Items[0].Title != "Kept" {
			t.Errorf("items = %+v", feed.Items)
		}
	})
	t.Run("an item with only a summary is called Untitled", func(t *testing.T) {
		feed := mustParse(t, "<rss><channel><item><description>Body text</description></item></channel></rss>", "")
		if len(feed.Items) != 1 || feed.Items[0].Title != "Untitled" || feed.Items[0].Summary != "Body text" {
			t.Errorf("items = %+v", feed.Items)
		}
	})
	t.Run("a feed without items has an empty list", func(t *testing.T) {
		feed := mustParse(t, "<rss><channel><title>Empty</title></channel></rss>", "")
		if feed.Items == nil || len(feed.Items) != 0 {
			t.Errorf("items = %#v", feed.Items)
		}
	})
	t.Run("a relative link with no base is dropped", func(t *testing.T) {
		feed, err := Parse([]byte("<rss><channel><item><title>T</title><link>/x</link></item></channel></rss>"), "", nil)
		if err != nil || feed.Items[0].Link != nil {
			t.Errorf("feed = %+v, err = %v", feed, err)
		}
	})
}

func TestParseRejectsNonFeeds(t *testing.T) {
	cases := []struct {
		name string
		body string
		want error
	}{
		{"html page", "<!DOCTYPE html><html><head><title>Hi</title></head><body></body></html>", ErrNotAFeed},
		{"plain text", "just some words", ErrNotAFeed},
		{"empty body", "", ErrNotAFeed},
		{"other xml", "<?xml version=\"1.0\"?><sitemap><url/></sitemap>", ErrNotAFeed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.body), "", nil); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
	t.Run("a truncated feed is an error", func(t *testing.T) {
		_, err := Parse([]byte("<rss><channel><item><title>Cut off"), "", nil)
		if err == nil || errors.Is(err, ErrNotAFeed) {
			t.Errorf("err = %v, want an xml error", err)
		}
	})
}

func TestParseDoesNotResolveExternalEntities(t *testing.T) {
	feed := mustParse(t, `<?xml version="1.0"?>
<!DOCTYPE rss [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
  <!ENTITY remote SYSTEM "http://127.0.0.1:1/leak">
]>
<rss><channel><title>T</title><item><title>Before &xxe; and &remote; after &copy;</title></item></channel></rss>`, "")
	got := feed.Items[0].Title
	if strings.Contains(got, "root:") || !strings.HasPrefix(got, "Before") || !strings.Contains(got, "©") {
		t.Errorf("title = %q", got)
	}
}
