package audit

import (
	"strings"
	"testing"
	"time"
)

type maskCase struct{ name, in, want string }

var maskCases = []maskCase{
	{"url userinfo", "SELECT * FROM url('https://user:one@host/x.csv')", "SELECT * FROM url('https://***@host/x.csv')"},
	{"url userinfo holding an at sign", `{app="a"} |= "postgres://admin:o@ne@db:5432/app"`, `{app="a"} |= "postgres://***@db:5432/app"`},
	{"url userinfo of a nested url", "https://host/p?next=https://a:b@other/ x@y.z", "https://host/p?next=https://***@other/ x@y.z"},
	{"one character of userinfo", "ftp://u@host/f", "ftp://*@host/f"},

	{"s3 keys", `SELECT * FROM s3('https://b.s3.amazonaws.com/x.csv', 'AKIAABCDEFGHIJKLMNOP', 'sEcR3tK3yValue', 'CSV')`, `SELECT * FROM s3('https://b.s3.amazonaws.com/x.csv', '***', '***', 'CSV')`},
	{"s3 in other case with double quotes and spaces", `S3 ( "https://b/x", "AK", "SK" , "Parquet")`, `S3 ( "https://b/x", "**", "**" , "Parquet")`},
	{"s3 session token", `s3('https://b/x', 'ak', 'sk', 'tokentokentoken', 'CSV')`, `s3('https://b/x', '**', '**', '***', 'CSV')`},
	{"s3 without keys keeps format and structure", `s3('https://b/x.csv', 'CSV', 'a UInt8, b String', 'gzip')`, `s3('https://b/x.csv', 'CSV', 'a UInt8, b String', 'gzip')`},
	{"s3 NOSIGN", `s3('https://b/x', NOSIGN, 'Parquet')`, `s3('https://b/x', NOSIGN, 'Parquet')`},
	{"s3Cluster", `s3Cluster('c1', 'https://b/x', 'ak12', 'sk12', 'CSV')`, `s3Cluster('c1', 'https://b/x', '***', '***', 'CSV')`},
	{"gcs", `gcs('https://storage.googleapis.com/b/x', 'hmackey', 'hmacsecret', 'CSV')`, `gcs('https://storage.googleapis.com/b/x', '***', '***', 'CSV')`},
	{"azure connection string", `azureBlobStorage('DefaultEndpointsProtocol=https;AccountName=a;AccountKey=zzzz;', 'c', 'b')`, `azureBlobStorage('***', 'c', 'b')`},
	{"azure account key", `azureBlobStorage('https://a.blob.core.windows.net', 'c', 'b', 'acct', 'accountkeyvalue', 'CSV')`, `azureBlobStorage('https://a.blob.core.windows.net', 'c', 'b', 'acct', '***', 'CSV')`},
	{"hdfs", `hdfs('hdfs://nn:9000/x', 'CSV')`, `hdfs('hdfs://nn:9000/x', 'CSV')`},
	{"url headers", `url('https://h/x', 'CSV', 'a String', headers('Authorization'='Bearer abcdef', 'X-Api'='k'))`, `url('https://h/x', 'CSV', 'a String', headers('Authorization'='***', 'X-Api'='*'))`},
	{"mysql", `SELECT * FROM mysql('h:3306', 'db', 't', 'root', 'hunter2')`, `SELECT * FROM mysql('h:3306', 'db', 't', 'root', '***')`},
	{"mysql engine", `CREATE TABLE t (a Int8) ENGINE = MySQL('h:3306', 'db', 't', 'root', 'hunter2')`, `CREATE TABLE t (a Int8) ENGINE = MySQL('h:3306', 'db', 't', 'root', '***')`},
	{"postgresql", `postgresql('h:5432', 'db', 't', 'u', 'hunter2', 'public')`, `postgresql('h:5432', 'db', 't', 'u', '***', 'public')`},
	{"mongodb", `mongodb('h:27017', 'db', 'c', 'u', 'hunter2', 'a String')`, `mongodb('h:27017', 'db', 'c', 'u', '***', 'a String')`},
	{"redis", `redis('h:6379', 'k', 'a String', 0, 'hunter2')`, `redis('h:6379', 'k', 'a String', 0, '***')`},
	{"remote with db.table", `remote('h', db.t, 'default', 'hunter2')`, `remote('h', db.t, 'default', '***')`},
	{"remoteSecure with db and table", `remoteSecure('h', 'db', 't', 'u', 'hunter2')`, `remoteSecure('h', 'db', 't', '*', '***')`},
	{"jdbc", `jdbc('jdbc:mysql://h/db?user=u&password=hunter2', 'x', 'y')`, `jdbc('***', 'x', 'y')`},
	{"odbc", `odbc('DSN=x;UID=u;PWD=hunter2', 'x', 'y')`, `odbc('***', 'x', 'y')`},
	{"named arguments", `s3(coll, access_key_id = 'AKIAABCDEFGHIJKLMNOP', secret_access_key='shhh', format='CSV')`, `s3(coll, access_key_id = '***', secret_access_key='***', format='CSV')`},
	{"a call cut inside its last literal", `SELECT * FROM mysql('h', 'db', 't', 'u', 'hunter2`, `SELECT * FROM mysql('h', 'db', 't', 'u', '***`},
	{"a call that never closes", `s3('https://b/x', 'ak', 'sk'`, `s3('https://b/x', '**', '**'`},

	{"password", "password=hunter2 and more", "password=*** and more"},
	{"quoted key and value", `{"api_key": "abcd1234", "ok": 1}`, `{"api_key": "***", "ok": 1}`},
	{"single quoted value in sql", `WHERE TOKEN = 'abc123'`, `WHERE TOKEN = '***'`},
	{"label matcher", `{app="x", token="abc123"}`, `{app="x", token="***"}`},
	{"short value", "pwd=ab", "pwd=**"},
	{"each key", "passwd=aaaa secret=bbbb apikey=cccc access_key=dddd secret_key=eeee", "passwd=*** secret=*** apikey=*** access_key=*** secret_key=***"},
	{"authorization header", `curl -H "Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123"`, `curl -H "Authorization: Bearer ***"`},
	{"quoted authorization", `authorization="Bearer sometoken123"`, `authorization="***"`},
	{"bearer in a log search", `{app="a"} |= "Bearer abcdefghijklmnop1234"`, `{app="a"} |= "Bearer ***"`},

	{"aws access key id", "key AKIAABCDEFGHIJKLMNOP used", "key *** used"},
	{"aws temporary key id", "ASIAABCDEFGHIJKLMNOP", "***"},
	{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl", "***"},
	{"jwt in a search", `|= "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"`, `|= "***"`},
	{"pem block", "x -----BEGIN RSA PRIVATE KEY-----\nMIIB\nabc\n-----END RSA PRIVATE KEY----- y", "x *** y"},
	{"pem block that never ends", "-----BEGIN PRIVATE KEY-----\nMIIBabc", "***"},
}

var untouchedQueries = []string{
	`sum(rate(http_requests_total{job="api",code=~"5.."}[5m])) by (instance)`,
	`{app="shop"} |= "error" | json | token_count > 10`,
	`traces | where duration > 1s and token_count == 5 | summarize count() by name`,
	`SELECT user, count() FROM logs WHERE ts > now() - INTERVAL 1 HOUR AND level = 'error' GROUP BY user`,
	`{token_count="5"}`,
	`{token=~"a.*"}`,
	`a == b and token != "x"`,
	`rate(secret_rotations_total[5m]) + password_changes_total`,
	`sum by (token_type) (tokens)`,
	`https://host/path?a=b&c=d`,
	`mail me@example.com about http://example.com/a`,
	`{app="a"} |= "bearer authentication failed"`,
	`up{url="http://example.com/a"}`,
	`SELECT * FROM s3('https://b/x.csv', 'CSV', 'a UInt8')`,
	`SELECT * FROM remote('h', 'db', 't')`,
	`AKIAABCDEFGHIJKLMNOPQ and AKIAshort and eyJabc.def`,
}

func TestMaskSecretsMasksEachCredentialForm(t *testing.T) {
	for _, c := range maskCases {
		t.Run(c.name, func(t *testing.T) {
			if got := maskSecrets(c.in); got != c.want {
				t.Errorf("maskSecrets(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
			if got := RecordedQuery(c.in); got != c.want {
				t.Errorf("RecordedQuery(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMaskSecretsLeavesOrdinaryQueriesAlone(t *testing.T) {
	for _, q := range untouchedQueries {
		if got := RecordedQuery(q); got != q {
			t.Errorf("RecordedQuery(%q) = %q, want it unchanged", q, got)
		}
	}
}

func TestMaskSecretsNeverLengthensAndNeverPanicsOnAnyPrefixOrGarbage(t *testing.T) {
	var texts []string
	for _, c := range maskCases {
		texts = append(texts, c.in)
	}
	texts = append(texts, untouchedQueries...)
	state := uint64(7)
	next := func(n int) int {
		state = state*6364136223846793005 + 1442695040888963407
		return int(state>>33) % n
	}
	pieces := []string{"s3(", "mysql(", "url(", "headers(", "remote(", "'", `"`, "`", "\\", ",", ")", "(", "=", ":", "@", "://", "password", "token", "Bearer ", "AKIA", "eyJ", ".", "-----BEGIN PRIVATE KEY-----", " ", "\n", "é", "a", "9"}
	for range 400 {
		var b strings.Builder
		for range next(60) {
			b.WriteString(pieces[next(len(pieces))])
		}
		texts = append(texts, b.String())
	}
	for _, text := range texts {
		for end := 0; end <= len(text); end++ {
			if got := maskSecrets(text[:end]); len(got) > end {
				t.Fatalf("maskSecrets(%q) = %q, longer than its input", text[:end], got)
			}
		}
	}
}

func TestMaskSecretsRunsInLinearTimeOnHostileInput(t *testing.T) {
	for name, unit := range map[string]string{
		"nested calls": "s3(", "open quotes": "mysql('", "keys": "password=", "headers": "url(headers(", "urls": "a://b", "pem": "-----BEGIN PRIVATE KEY-----",
	} {
		start := time.Now()
		got := RecordedQuery(strings.Repeat(unit, (1<<20)/len(unit)))
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("%s: a 1 MiB query took %s", name, took)
		}
		if len(got) > MaxQueryBytes+len(truncatedMark) {
			t.Errorf("%s: recorded %d bytes", name, len(got))
		}
	}
}

func TestASecretThatStraddlesTheCapDoesNotSurvive(t *testing.T) {
	pad := strings.Repeat("a", MaxQueryBytes-21) + " "
	for name, tail := range map[string]string{
		"a table function": "mysql('h', 'd', 't', 'u', 'pw1234567890abcdefghijklmnop')",
		"a url":            "s3://k:SECRETKEY1234567890@bucket",
		"a key":            "password=SECRETKEY1234567890",
		"a pem block":      "-----BEGIN PRIVATE KEY-----\nSECRETKEY1234567890\n-----END PRIVATE KEY-----",
	} {
		if got := RecordedQuery(pad + tail); strings.Contains(got, "SECRET") || strings.Contains(got, "pw1234") || strings.Contains(got, "1234567890") {
			t.Errorf("%s survived the cap: %q", name, got[MaxQueryBytes-30:])
		}
	}
}
