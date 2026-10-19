package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const validPrometheus = `
datasources:
  - uid: prom
    type: prometheus
    name: Prometheus
    prometheus:
      url: http://prometheus:9090
`

func envOf(values map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := values[k]
		return v, ok
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, content string, env map[string]string) ([]Datasource, error) {
	t.Helper()
	return loadDatasources(writeFile(t, "datasources.yaml", content), envOf(env))
}

func wantError(t *testing.T, content string, env map[string]string, substrings ...string) {
	t.Helper()
	_, err := load(t, content, env)
	if err == nil {
		t.Fatalf("expected an error containing %q, got none", substrings)
	}
	for _, s := range substrings {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not contain %q", err, s)
		}
	}
}

func prometheusWith(fields string) string {
	return validPrometheus + indent(fields, "      ")
}

func indent(block, prefix string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.Trim(block, "\n"), "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

func clickhouseWith(fields string) string {
	return `
datasources:
  - uid: ch
    type: clickhouse
    name: ClickHouse
    clickhouse:
      addr: clickhouse:9000
      username: ro
` + indent(fields, "      ")
}

func TestLoadDatasourcesValid(t *testing.T) {
	got, err := load(t, validPrometheus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UID != "prom" || got[0].Prometheus == nil || got[0].Prometheus.URL != "http://prometheus:9090" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func promWithAliases(aliases string) string {
	return validPrometheus + "    grafanaUids: " + aliases + "\n"
}

func secondPrometheus(uid, aliases string) string {
	return fmt.Sprintf("  - uid: %s\n    type: prometheus\n    grafanaUids: %s\n    prometheus:\n      url: http://other:9090\n", uid, aliases)
}

func errorLines(t *testing.T, content string) []string {
	t.Helper()
	_, err := load(t, content, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	return strings.Split(err.Error(), "\n")
}

func TestLoadDatasourcesStructure(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"unknown field", prometheusWith("password: hunter2"), `unknown field "password"`},
		{"duplicate key", "datasources:\n  - uid: a\n    uid: b\n", "already defined"},
		{"multiple documents", validPrometheus + "---\n" + validPrometheus, "multiple YAML documents are not supported"},
		{"second document with other content", validPrometheus + "---\nfoo: bar\n", "multiple YAML documents are not supported"},
		{"missing key", "{}\n", "datasources: required"},
		{"empty file", "", "datasources: required"},
		{"null list", "datasources:\n", "datasources: required"},
		{"not yaml", "datasources: [", "invalid datasources file"},
		{"duplicate uid", validPrometheus + indent(`
- uid: prom
  type: prometheus
  prometheus:
    url: http://other:9090
`, "  "), `datasources[1].uid: duplicate "prom"`},
		{"invalid uid", strings.Replace(validPrometheus, "uid: prom", "uid: -bad", 1), "datasources[0].uid: must match"},
		{"unknown type", strings.Replace(validPrometheus, "type: prometheus", "type: loki", 1), "datasources[0].type"},
		{"missing block", "datasources:\n  - uid: a\n    type: prometheus\n", "datasources[0].prometheus: required"},
		{"wrong block", validPrometheus + "    clickhouse:\n      addr: x\n", "datasources[0].clickhouse: not allowed"},
		{"negative limit", validPrometheus + "    limits:\n      maxRows: -1\n", "datasources[0].limits.maxRows: must not be negative"},
		{"bad duration", prometheusWith("scrapeInterval: soon"), "datasources[0].prometheus.scrapeInterval: invalid duration"},
		{"blank alias", promWithAliases(`["  "]`), "datasources[0].grafanaUids[0]: must not be empty"},
		{"null alias", promWithAliases("[~]"), "datasources[0].grafanaUids[0]: must not be empty"},
		{"alias over 100 bytes", promWithAliases(`["` + strings.Repeat("a", 101) + `"]`), "datasources[0].grafanaUids[0]: must be at most 100 bytes"},
		{"alias with control character", promWithAliases(`["a\tb"]`), "datasources[0].grafanaUids[0]: must not contain control characters"},
		{"alias with zero width space", promWithAliases(`["a\u200Bb"]`), "datasources[0].grafanaUids[0]: must not contain invisible characters"},
		{"alias with byte order mark", promWithAliases(`["a\uFEFFb"]`), "datasources[0].grafanaUids[0]: must not contain invisible characters"},
		{"alias with line separator", promWithAliases(`["a\u2028b"]`), "datasources[0].grafanaUids[0]: must not contain invisible characters"},
		{"alias with paragraph separator", promWithAliases(`["a\u2029b"]`), "datasources[0].grafanaUids[0]: must not contain invisible characters"},
		{"too many aliases", promWithAliases("[" + manyAliases(21) + "]"), "datasources[0].grafanaUids: at most 20 entries"},
		{"duplicate alias after trimming", promWithAliases(`[a, " a"]`), `datasources[0].grafanaUids[1]: duplicate "a"`},
		{"alias owned by another datasource", validPrometheus + secondPrometheus("second", "[a]") + secondPrometheus("third", "[a]"), `datasources[2].grafanaUids[0]: "a" is already listed by datasource "second"`},
		{"alias owned by a datasource with an invalid uid", "datasources:\n" + secondPrometheus("-bad", "[a]") + secondPrometheus("second", "[a]"), `datasources[1].grafanaUids[0]: "a" is already listed by datasource datasources[0]`},
		{"alias owned by a datasource with a duplicated uid", validPrometheus + secondPrometheus("prom", "[a]") + secondPrometheus("third", "[a]"), `datasources[2].grafanaUids[0]: "a" is already listed by datasource datasources[1]`},
		{"alias equal to another datasource uid", validPrometheus + secondPrometheus("second", "[prom]"), `datasources[1].grafanaUids[0]: "prom" is the uid of datasource "prom"`},
		{"alias equal to a later datasource uid", "datasources:\n" + secondPrometheus("first", "[second]") + secondPrometheus("second", "[]"), `datasources[0].grafanaUids[0]: "second" is the uid of datasource "second"`},
		{"numeric alias", promWithAliases("[0123]"), "datasources[0].grafanaUids[0]: must be a string (quote it)"},
		{"boolean alias", promWithAliases("[true]"), "datasources[0].grafanaUids[0]: must be a string (quote it)"},
		{"nested alias", promWithAliases("[[a]]"), "datasources[0].grafanaUids[0]: must be a string"},
		{"numeric uid", strings.Replace(validPrometheus, "uid: prom", "uid: 0123", 1), "datasources[0].uid: must be a string (quote it)"},
		{"boolean uid", strings.Replace(validPrometheus, "uid: prom", "uid: true", 1), "datasources[0].uid: must be a string (quote it)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantError(t, tt.content, nil, tt.want)
		})
	}
}

func manyAliases(n int) string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("g%d", i)
	}
	return strings.Join(names, ", ")
}

func TestGrafanaUIDsAreTrimmedAndKeepCase(t *testing.T) {
	got, err := load(t, promWithAliases(`[" a ", B]`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "B"}; !slices.Equal(got[0].GrafanaUIDs, want) {
		t.Fatalf("grafanaUids = %q, want %q", got[0].GrafanaUIDs, want)
	}
}

func TestGrafanaUIDLimits(t *testing.T) {
	longest := strings.Repeat("a", 100)
	got, err := load(t, promWithAliases(`["`+longest+`"]`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got[0].GrafanaUIDs, []string{longest}) {
		t.Fatalf("grafanaUids = %q", got[0].GrafanaUIDs)
	}
	got, err = load(t, promWithAliases("["+manyAliases(20)+"]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].GrafanaUIDs) != 20 {
		t.Fatalf("got %d aliases", len(got[0].GrafanaUIDs))
	}
}

func TestGrafanaUIDEqualToOwnUIDIsAllowed(t *testing.T) {
	got, err := load(t, promWithAliases("[prom]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got[0].GrafanaUIDs, []string{"prom"}) {
		t.Fatalf("grafanaUids = %q", got[0].GrafanaUIDs)
	}
}

func TestQuotedNumbersAreKeptVerbatim(t *testing.T) {
	content := strings.Replace(validPrometheus, "uid: prom", `uid: "0123"`, 1) + `    grafanaUids: ["0123", "00012", "0x1F", "1_000", "1.50"]` + "\n"
	got, err := load(t, content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].UID != "0123" {
		t.Fatalf("uid = %q", got[0].UID)
	}
	if want := []string{"0123", "00012", "0x1F", "1_000", "1.50"}; !slices.Equal(got[0].GrafanaUIDs, want) {
		t.Fatalf("grafanaUids = %q, want %q", got[0].GrafanaUIDs, want)
	}
}

func TestUnquotedNumbersAreRefusedNotRewritten(t *testing.T) {
	lines := errorLines(t, promWithAliases("[0123, 00012, 0x1F, 1_000, 1.50]"))
	if len(lines) != 5 {
		t.Fatalf("want 5 errors, got %q", lines)
	}
	for j, line := range lines {
		want := fmt.Sprintf("datasources[0].grafanaUids[%d]: must be a string (quote it)", j)
		if line != want {
			t.Errorf("line %d = %q, want %q", j, line, want)
		}
	}
	for _, literal := range []string{"0123", "0x1F", "1.50", "1_000"} {
		lines := errorLines(t, strings.Replace(validPrometheus, "uid: prom", "uid: "+literal, 1))
		if want := "datasources[0].uid: must be a string (quote it)"; len(lines) != 1 || lines[0] != want {
			t.Errorf("uid %s: errors = %q, want only %q", literal, lines, want)
		}
	}
}

func TestEachGrafanaUIDEntryReportsOneError(t *testing.T) {
	lines := errorLines(t, validPrometheus+indent("grafanaUids: [a]", "    ")+secondPrometheus("second", "[a, a]"))
	if len(lines) != 2 {
		t.Fatalf("want 2 errors, got %q", lines)
	}
	if want := `datasources[1].grafanaUids[1]: duplicate "a"`; lines[0] != want {
		t.Errorf("first error = %q, want %q", lines[0], want)
	}
	if want := `datasources[1].grafanaUids[0]: "a" is already listed by datasource "prom"`; lines[1] != want {
		t.Errorf("second error = %q, want %q", lines[1], want)
	}
}

func TestDeployDatasourceFilesLoad(t *testing.T) {
	env := map[string]string{"SLASHOBSERVE_CLICKHOUSE_PASSWORD": "pw"}
	for _, name := range []string{"datasources.yaml", "datasources.e2e.yaml"} {
		t.Run(name, func(t *testing.T) {
			got, err := loadDatasources(filepath.Join("..", "..", "deploy", "compose", name), envOf(env))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 {
				t.Fatal("expected datasources")
			}
			if name != "datasources.e2e.yaml" {
				return
			}
			aliases := map[string][]string{}
			for _, ds := range got {
				aliases[ds.UID] = ds.GrafanaUIDs
			}
			if !slices.Equal(aliases["prom"], []string{"prometheus"}) || !slices.Equal(aliases["ch"], []string{"clickhouse"}) {
				t.Fatalf("aliases = %v", aliases)
			}
		})
	}
}

func TestLoadDatasourcesEmptyListIsAllowed(t *testing.T) {
	got, err := load(t, "datasources: []\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no datasources, got %+v", got)
	}
}

func TestLimitsMustBeIntegers(t *testing.T) {
	for _, literal := range []string{"-0.5", "1.5", "1e3", `"12"`, "true"} {
		t.Run(literal, func(t *testing.T) {
			_, err := load(t, validPrometheus+"    limits:\n      maxSeries: "+literal+"\n", nil)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	for _, literal := range []string{"-0.5", "1.5", "1e3", `"12"`} {
		wantError(t, validPrometheus+"    limits:\n      maxRows: "+literal+"\n", nil, "a limits value must be an integer")
	}
	got, err := load(t, validPrometheus+"    limits:\n      maxSeries: 12\n      maxPoints: 0\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Limits.MaxSeries != 12 || got[0].Limits.MaxPoints != 0 {
		t.Fatalf("limits: %+v", got[0].Limits)
	}
	wantError(t, validPrometheus+"    limits:\n      maxRows: -3\n", nil, "limits.maxRows: must not be negative")
}

func TestOverflowErrorDoesNotEchoLiteral(t *testing.T) {
	const literal = "99999999999999999999"
	_, err := load(t, validPrometheus+"    limits:\n      maxSeries: "+literal+"\n", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), literal) {
		t.Fatalf("error echoes the literal: %v", err)
	}
	if !strings.Contains(err.Error(), "line 9 column 18: number out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnvSecrets(t *testing.T) {
	t.Run("resolved", func(t *testing.T) {
		got, err := load(t, clickhouseWith("passwordEnv: SLASHOBSERVE_CH_PASSWORD"), map[string]string{"SLASHOBSERVE_CH_PASSWORD": "s3cret"})
		if err != nil {
			t.Fatal(err)
		}
		if got[0].ClickHouse.Password.Reveal() != "s3cret" {
			t.Fatal("password not resolved")
		}
	})
	t.Run("prefix required", func(t *testing.T) {
		wantError(t, clickhouseWith("passwordEnv: HOME"), map[string]string{"HOME": "x"}, "datasources[0].clickhouse.passwordEnv: must start with SLASHOBSERVE_")
	})
	t.Run("unset", func(t *testing.T) {
		wantError(t, clickhouseWith("passwordEnv: SLASHOBSERVE_CH_PASSWORD"), nil, "SLASHOBSERVE_CH_PASSWORD is not set or empty")
	})
	t.Run("empty", func(t *testing.T) {
		wantError(t, clickhouseWith("passwordEnv: SLASHOBSERVE_CH_PASSWORD"), map[string]string{"SLASHOBSERVE_CH_PASSWORD": ""}, "is not set or empty")
	})
	t.Run("none given", func(t *testing.T) {
		wantError(t, clickhouseWith("tls: false"), nil, "one of passwordEnv or passwordFile is required")
	})
}

func TestServerOwnedEnvNamesAreDenied(t *testing.T) {
	env := map[string]string{}
	for _, name := range serverEnvNames {
		env[name] = "server-owned-secret"
	}
	for _, name := range serverEnvNames {
		t.Run(name, func(t *testing.T) {
			cases := map[string]string{
				"password": clickhouseWith("passwordEnv: " + name),
				"bearer":   prometheusWith("bearerTokenEnv: " + name),
				"basic":    prometheusWith("basicAuth:\n  username: u\n  passwordEnv: " + name),
				"header":   prometheusWith("headersFromEnv:\n  X-Key: " + name),
			}
			for label, content := range cases {
				_, err := load(t, content, env)
				if err == nil {
					t.Fatalf("%s: expected an error", label)
				}
				if !strings.Contains(err.Error(), name+" is reserved for the server's own configuration") {
					t.Errorf("%s: unexpected error %q", label, err)
				}
				if strings.Contains(err.Error(), "server-owned-secret") {
					t.Errorf("%s: error echoes the value", label)
				}
			}
		})
	}
}

func TestServerEnvNamesCoverEveryNameConfigReads(t *testing.T) {
	literal := regexp.MustCompile(`"(SLASHOBSERVE_[A-Z0-9_]+)"`)
	for _, file := range []string{"config.go", "datasources.go"} {
		src, err := os.ReadFile(file) //nolint:gosec // fixed test file names
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range literal.FindAllStringSubmatch(string(src), -1) {
			if !slices.Contains(serverEnvNames, m[1]) {
				t.Errorf("%s is read in %s but missing from serverEnvNames", m[1], file)
			}
		}
	}
}

func TestMutualExclusivity(t *testing.T) {
	env := map[string]string{"SLASHOBSERVE_A": "x"}
	secretFile := writeFile(t, "secret", "x\n")
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"password env and file", clickhouseWith("passwordEnv: SLASHOBSERVE_A\npasswordFile: " + secretFile), "passwordEnv and passwordFile are mutually exclusive"},
		{"bearer env and file", prometheusWith("bearerTokenEnv: SLASHOBSERVE_A\nbearerTokenFile: " + secretFile), "bearerTokenEnv and bearerTokenFile are mutually exclusive"},
		{"basic and bearer", prometheusWith("bearerTokenEnv: SLASHOBSERVE_A\nbasicAuth:\n  username: u\n  passwordEnv: SLASHOBSERVE_A"), "basicAuth and bearerToken are mutually exclusive"},
		{"basic needs username", prometheusWith("basicAuth:\n  passwordEnv: SLASHOBSERVE_A"), "basicAuth.username: required"},
		{"header in plain and env", prometheusWith("headers:\n  x-key: v\nheadersFromEnv:\n  X-Key: SLASHOBSERVE_A"), `header "X-Key" is set more than once`},
		{"header differing only by case in plain headers", prometheusWith("headers:\n  x-k: a\n  X-K: b"), `datasources[0].prometheus.headers[x-k]: header "X-K" is set more than once`},
		{"header in env and file", prometheusWith("headersFromEnv:\n  X-Key: SLASHOBSERVE_A\nheadersFromFile:\n  X-Key: " + secretFile), `header "X-Key" is set more than once`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantError(t, tt.content, env, tt.want)
		})
	}
}

func TestSecretFiles(t *testing.T) {
	t.Run("trimmed", func(t *testing.T) {
		path := writeFile(t, "token", "tok\r\n")
		got, err := load(t, prometheusWith("bearerTokenFile: "+path), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Prometheus.BearerToken.Reveal() != "tok" {
			t.Fatal("token not trimmed")
		}
	})
	t.Run("empty", func(t *testing.T) {
		path := writeFile(t, "token", "\n")
		wantError(t, prometheusWith("bearerTokenFile: "+path), nil, "bearerTokenFile: "+path+" is empty")
	})
	t.Run("missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent")
		wantError(t, clickhouseWith("passwordFile: "+path), nil, "datasources[0].clickhouse.passwordFile:")
	})
}

func TestHeadersFromEnvAndFile(t *testing.T) {
	file := writeFile(t, "key", "file-value\n")
	got, err := load(t, prometheusWith("headers:\n  X-Plain: p\nheadersFromEnv:\n  X-Env: SLASHOBSERVE_HDR\nheadersFromFile:\n  X-File: "+file), map[string]string{"SLASHOBSERVE_HDR": "env-value"})
	if err != nil {
		t.Fatal(err)
	}
	p := got[0].Prometheus
	if p.Headers["X-Plain"] != "p" || len(p.Headers) != 1 {
		t.Fatalf("plain headers: %v", p.Headers)
	}
	if p.SecretHeaders["X-Env"].Reveal() != "env-value" || p.SecretHeaders["X-File"].Reveal() != "file-value" || len(p.SecretHeaders) != 2 {
		t.Fatalf("secret headers: %v", p.SecretHeaders)
	}
}

func TestHeaderSecretErrors(t *testing.T) {
	wantError(t, prometheusWith("headersFromEnv:\n  X-Env: SLASHOBSERVE_MISSING"), nil, "datasources[0].prometheus.headersFromEnv[X-Env]: SLASHOBSERVE_MISSING is not set or empty")
	wantError(t, prometheusWith("headersFromEnv:\n  X-Env: PATH"), nil, "headersFromEnv[X-Env]: must start with SLASHOBSERVE_")
	empty := writeFile(t, "empty", "")
	wantError(t, prometheusWith("headersFromFile:\n  X-File: "+empty), nil, "headersFromFile[X-File]: "+empty+" is empty")
}

func TestErrorsNeverEchoSecrets(t *testing.T) {
	const secret = "top-secret-value"
	env := map[string]string{"SLASHOBSERVE_S": secret}
	file := writeFile(t, "secret", secret)
	contents := []string{
		prometheusWith("bearerTokenEnv: SLASHOBSERVE_S\nbearerTokenFile: " + file + "\nurl: " + secret),
		prometheusWith("bearerTokenEnv: SLASHOBSERVE_S\nbasicAuth:\n  passwordEnv: SLASHOBSERVE_S"),
		prometheusWith("headers:\n  X-K: v\nheadersFromEnv:\n  X-K: SLASHOBSERVE_S\nheadersFromFile:\n  X-K: " + file),
		clickhouseWith("passwordEnv: SLASHOBSERVE_S\npasswordFile: " + file + "\ntls: " + secret),
		prometheusWith("bearerTokenEnv: SLASHOBSERVE_DATABASE_URL"),
	}
	env["SLASHOBSERVE_DATABASE_URL"] = secret
	for i, content := range contents {
		_, err := load(t, content, env)
		if err == nil {
			t.Fatalf("case %d: expected an error", i)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("case %d: error echoes a secret: %v", i, err)
		}
	}
}
