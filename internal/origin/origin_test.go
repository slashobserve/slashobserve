package origin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	zeroWidthSpace  = "\xe2\x80\x8b"
	zeroWidthNonJoi = "\xe2\x80\x8c"
	zeroWidthJoiner = "\xe2\x80\x8d"
	wordJoiner      = "\xe2\x81\xa0"
	byteOrderMark   = "\xef\xbb\xbf"
	bidiOverride    = "\xe2\x80\xae"
	bidiPopFormat   = "\xe2\x80\xac"
	bidiIsolate     = "\xe2\x81\xa6"
	bidiPopIsolate  = "\xe2\x81\xa9"
	noBreakSpace    = "\xc2\xa0"
	emSpace         = "\xe2\x80\x83"
	nextLine        = "\xc2\x85"
	eAcute          = "\xc3\xa9"
	han             = "\xe6\x97\xa5"
)

func TestCleanClient(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "claude-code", "claude-code"},
		{"inner spaces are kept", "Claude  Code", "Claude  Code"},
		{"surrounding whitespace", "  cursor \t\n", "cursor"},
		{"empty", "", ""},
		{"whitespace only", " \t " + noBreakSpace + emSpace, ""},
		{"NUL", "a\x00b", "ab"},
		{"C0 and C1 controls", "a\x1b\x07b" + nextLine + "c\x7f", "abc"},
		{"zero-width space and joiners", "a" + zeroWidthSpace + "b" + zeroWidthNonJoi + "c" + zeroWidthJoiner + "d" + wordJoiner + "e" + byteOrderMark, "abcde"},
		{"bidi override and isolates", bidiOverride + "txt." + bidiPopFormat + bidiIsolate + "x" + bidiPopIsolate, "txt.x"},
		{"only invisible characters", zeroWidthSpace + bidiOverride + "\x00", ""},
		{"invalid UTF-8", "a\xff\xfeb\xc3", "ab"},
		{"only invalid UTF-8", "\xff\xfe", ""},
		{"multibyte kept", eAcute + han, eAcute + han},
		{"exactly 64 runes", strings.Repeat(eAcute, 64), strings.Repeat(eAcute, 64)},
		{"65 runes cut to 64", strings.Repeat(eAcute, 65), strings.Repeat(eAcute, 64)},
		{"cut by runes not bytes", strings.Repeat(han, 100), strings.Repeat(han, 64)},
		{"trailing space exposed by the cut is trimmed", strings.Repeat("a", 63) + " b", strings.Repeat("a", 63)},
		{"invisible characters do not count toward the cut", strings.Repeat("a"+zeroWidthSpace, 70), strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanClient(tt.in); got != tt.want {
				t.Errorf("CleanClient(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestForPrincipal(t *testing.T) {
	user := auth.Principal{Kind: auth.KindUser, ID: "u"}
	account := auth.Principal{Kind: auth.KindServiceAccount, ID: "s"}
	tests := []struct {
		name      string
		principal auth.Principal
		userAgent string
		want      Origin
	}{
		{"user is web", user, "Mozilla/5.0", Origin{Kind: Web}},
		{"user with a CLI user agent is still web", user, "observe/1.0", Origin{Kind: Web}},
		{"service account with the CLI user agent", account, "observe/1.2.3", Origin{Kind: CLI}},
		{"service account with the bare CLI prefix", account, "observe/", Origin{Kind: CLI}},
		{"service account with another agent", account, "curl/8.0", Origin{Kind: API}},
		{"service account with no user agent", account, "", Origin{Kind: API}},
		{"prefix must be at the start", account, "my-observe/1.0", Origin{Kind: API}},
		{"prefix is case sensitive", account, "Observe/1.0", Origin{Kind: API}},
		{"prefix needs the slash", account, "observe", Origin{Kind: API}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForPrincipal(tt.principal, tt.userAgent); got != tt.want {
				t.Errorf("ForPrincipal = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		principal *auth.Principal
		userAgent string
		want      Origin
	}{
		{"user", &auth.Principal{Kind: auth.KindUser, ID: "u"}, "observe/1", Origin{Kind: Web}},
		{"CLI key", &auth.Principal{Kind: auth.KindServiceAccount, ID: "s"}, "observe/1", Origin{Kind: CLI}},
		{"API key", &auth.Principal{Kind: auth.KindServiceAccount, ID: "s"}, "curl/8", Origin{Kind: API}},
		{"no principal leaves the origin unset", nil, "observe/1", Origin{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if tt.principal != nil {
					c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), *tt.principal))
				}
			}, Middleware)
			var got Origin
			engine.GET("/", func(c *gin.Context) {
				got = From(c.Request.Context())
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status %d", rec.Code)
			}
			if got != tt.want {
				t.Errorf("origin %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestColumns(t *testing.T) {
	tests := []struct {
		name      string
		in        Origin
		kind      string
		kindValid bool
		client    string
		clientSet bool
	}{
		{"unset", Origin{}, "", false, "", false},
		{"web", Origin{Kind: Web}, "web", true, "", false},
		{"web drops a client", Origin{Kind: Web, Client: "x"}, "web", true, "", false},
		{"cli drops a client", Origin{Kind: CLI, Client: "x"}, "cli", true, "", false},
		{"api drops a client", Origin{Kind: API, Client: "x"}, "api", true, "", false},
		{"mcp without a client", Origin{Kind: MCP}, "mcp", true, "", false},
		{"mcp with a client", Origin{Kind: MCP, Client: "claude-code"}, "mcp", true, "claude-code", true},
		{"no kind drops a client", Origin{Client: "x"}, "", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, client := tt.in.Columns()
			if kind.Valid != tt.kindValid || (kind.Valid && kind.String != tt.kind) {
				t.Errorf("kind %+v, want %q valid=%v", kind, tt.kind, tt.kindValid)
			}
			if client.Valid != tt.clientSet || (client.Valid && client.String != tt.client) {
				t.Errorf("client %+v, want %q valid=%v", client, tt.client, tt.clientSet)
			}
		})
	}
}
