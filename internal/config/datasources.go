package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/goccy/go-yaml"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	secretEnvPrefix = "SLASHOBSERVE_"

	overflowSuffix = "( overflow )"

	typePrometheus = "prometheus"
	typeClickHouse = "clickhouse"

	maxGrafanaUIDs     = 20
	maxGrafanaUIDBytes = 100
)

type Datasource struct {
	UID, Type, Name string
	GrafanaUIDs     []string
	Prometheus      *PrometheusDatasource
	ClickHouse      *ClickHouseDatasource
	Limits          datasource.Limits
}

type PrometheusDatasource struct {
	URL               string
	ScrapeInterval    time.Duration
	Headers           map[string]string
	SecretHeaders     map[string]datasource.Secret
	BasicAuthUser     string
	BasicAuthPassword datasource.Secret
	BearerToken       datasource.Secret
}

type ClickHouseDatasource struct {
	Addr, Database, Username string
	Password                 datasource.Secret
	TLS                      bool
}

type datasourcesFile struct {
	Datasources *[]fileDatasource `yaml:"datasources"`
}

type fileDatasource struct {
	UID         string          `yaml:"uid"`
	Type        string          `yaml:"type"`
	Name        string          `yaml:"name"`
	GrafanaUIDs []string        `yaml:"grafanaUids"`
	Prometheus  *filePrometheus `yaml:"prometheus"`
	ClickHouse  *fileClickHouse `yaml:"clickhouse"`
	Limits      fileLimits      `yaml:"limits"`
}

type filePrometheus struct {
	URL             string            `yaml:"url"`
	ScrapeInterval  string            `yaml:"scrapeInterval"`
	Headers         map[string]string `yaml:"headers"`
	HeadersFromEnv  map[string]string `yaml:"headersFromEnv"`
	HeadersFromFile map[string]string `yaml:"headersFromFile"`
	BasicAuth       *fileBasicAuth    `yaml:"basicAuth"`
	BearerTokenEnv  string            `yaml:"bearerTokenEnv"`
	BearerTokenFile string            `yaml:"bearerTokenFile"`
}

type fileBasicAuth struct {
	Username     string `yaml:"username"`
	PasswordEnv  string `yaml:"passwordEnv"`
	PasswordFile string `yaml:"passwordFile"`
}

type fileClickHouse struct {
	Addr         string `yaml:"addr"`
	Database     string `yaml:"database"`
	Username     string `yaml:"username"`
	PasswordEnv  string `yaml:"passwordEnv"`
	PasswordFile string `yaml:"passwordFile"`
	TLS          bool   `yaml:"tls"`
}

type fileLimits struct {
	Timeout   string      `yaml:"timeout"`
	MaxSeries wholeNumber `yaml:"maxSeries"`
	MaxPoints wholeNumber `yaml:"maxPoints"`
	MaxRows   wholeNumber `yaml:"maxRows"`
}

var errNotInteger = errors.New("a limits value must be an integer")

type wholeNumber int

func (n *wholeNumber) UnmarshalYAML(unmarshal func(any) error) error {
	var v int
	if err := unmarshal(&v); err != nil {
		return err
	}
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch raw.(type) {
	case int, int64, uint64:
		*n = wholeNumber(v)
		return nil
	default:
		return errNotInteger
	}
}

func loadDatasources(path string, lookup func(string) (string, bool)) ([]Datasource, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path is operator configuration
	if err != nil {
		return nil, err
	}

	var doc datasourcesFile
	decoder := yaml.NewDecoder(bytes.NewReader(content), yaml.DisallowUnknownField())
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid datasources file: %s", decodeMessage(err))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid datasources file: multiple YAML documents are not supported")
	}
	if doc.Datasources == nil {
		return nil, errors.New("invalid datasources file: datasources: required")
	}

	var errs []error
	seen := map[string]bool{}
	type aliasOwner struct {
		uid   string
		index int
	}
	aliasOwners := map[string]aliasOwner{}
	out := make([]Datasource, 0, len(*doc.Datasources))
	for i, raw := range *doc.Datasources {
		entryPath := fmt.Sprintf("datasources[%d]", i)
		ds, dsErrs := convertDatasource(entryPath, raw, lookup)
		errs = append(errs, dsErrs...)
		if datasource.ValidUID(raw.UID) {
			if seen[raw.UID] {
				errs = append(errs, fmt.Errorf("%s.uid: duplicate %q", entryPath, raw.UID))
			}
			seen[raw.UID] = true
		}
		for j, alias := range raw.GrafanaUIDs {
			alias = strings.TrimSpace(alias)
			if grafanaUIDProblem(alias) != "" {
				continue
			}
			owner, taken := aliasOwners[alias]
			switch {
			case !taken:
				aliasOwners[alias] = aliasOwner{uid: raw.UID, index: i}
			case owner.index != i:
				errs = append(errs, fmt.Errorf("%s.grafanaUids[%d]: %q is already listed by datasource %q", entryPath, j, alias, owner.uid))
			}
		}
		out = append(out, ds)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeMessage(err error) string {
	if errors.Is(err, errNotInteger) {
		return errNotInteger.Error()
	}
	var yerr yaml.Error
	if errors.As(err, &yerr) {
		msg := yerr.GetMessage()
		if strings.HasSuffix(msg, overflowSuffix) {
			msg = "number out of range"
		}
		if tok := yerr.GetToken(); tok != nil && tok.Position != nil {
			return fmt.Sprintf("line %d column %d: %s", tok.Position.Line, tok.Position.Column, msg)
		}
		return msg
	}
	return "cannot parse"
}

func convertDatasource(path string, raw fileDatasource, lookup func(string) (string, bool)) (Datasource, []error) {
	var errs []error
	fail := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s.%s: %s", path, field, fmt.Sprintf(format, args...)))
	}

	ds := Datasource{UID: raw.UID, Type: raw.Type, Name: raw.Name}
	if !datasource.ValidUID(raw.UID) {
		fail("uid", "must match %s", datasource.UIDPattern)
	}

	ds.GrafanaUIDs = convertGrafanaUIDs(path, raw.GrafanaUIDs, &errs)

	limits, limitErrs := convertLimits(path+".limits", raw.Limits)
	errs = append(errs, limitErrs...)
	ds.Limits = limits

	switch raw.Type {
	case typePrometheus:
		if raw.ClickHouse != nil {
			fail("clickhouse", "not allowed for type %s", typePrometheus)
		}
		if raw.Prometheus == nil {
			fail("prometheus", "required for type %s", typePrometheus)
			break
		}
		p, pErrs := convertPrometheus(path+".prometheus", *raw.Prometheus, lookup)
		errs = append(errs, pErrs...)
		ds.Prometheus = &p
	case typeClickHouse:
		if raw.Prometheus != nil {
			fail("prometheus", "not allowed for type %s", typeClickHouse)
		}
		if raw.ClickHouse == nil {
			fail("clickhouse", "required for type %s", typeClickHouse)
			break
		}
		c, cErrs := convertClickHouse(path+".clickhouse", *raw.ClickHouse, lookup)
		errs = append(errs, cErrs...)
		ds.ClickHouse = &c
	default:
		fail("type", "must be %s or %s, got %q", typePrometheus, typeClickHouse, raw.Type)
	}
	return ds, errs
}

func convertGrafanaUIDs(path string, raw []string, errs *[]error) []string {
	if len(raw) > maxGrafanaUIDs {
		*errs = append(*errs, fmt.Errorf("%s.grafanaUids: at most %d entries", path, maxGrafanaUIDs))
	}
	var out []string
	seen := map[string]bool{}
	for j, alias := range raw {
		alias = strings.TrimSpace(alias)
		if problem := grafanaUIDProblem(alias); problem != "" {
			*errs = append(*errs, fmt.Errorf("%s.grafanaUids[%d]: %s", path, j, problem))
			continue
		}
		if seen[alias] {
			*errs = append(*errs, fmt.Errorf("%s.grafanaUids[%d]: duplicate %q", path, j, alias))
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	return out
}

func grafanaUIDProblem(alias string) string {
	switch {
	case alias == "":
		return "must not be empty"
	case len(alias) > maxGrafanaUIDBytes:
		return fmt.Sprintf("must be at most %d bytes", maxGrafanaUIDBytes)
	case strings.ContainsFunc(alias, unicode.IsControl):
		return "must not contain control characters"
	}
	return ""
}

func convertLimits(path string, raw fileLimits) (datasource.Limits, []error) {
	var errs []error
	for _, f := range []struct {
		name  string
		value wholeNumber
	}{{"maxSeries", raw.MaxSeries}, {"maxPoints", raw.MaxPoints}, {"maxRows", raw.MaxRows}} {
		if f.value < 0 {
			errs = append(errs, fmt.Errorf("%s.%s: must not be negative", path, f.name))
		}
	}
	timeout, err := parseOptionalDuration(raw.Timeout)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s.timeout: %w", path, err))
	}
	return datasource.Limits{Timeout: timeout, MaxSeries: int(raw.MaxSeries), MaxPoints: int(raw.MaxPoints), MaxRows: int(raw.MaxRows)}, errs
}

func convertPrometheus(path string, raw filePrometheus, lookup func(string) (string, bool)) (PrometheusDatasource, []error) {
	var errs []error
	out := PrometheusDatasource{URL: raw.URL, Headers: raw.Headers}
	if raw.URL == "" {
		errs = append(errs, fmt.Errorf("%s.url: required", path))
	}
	interval, err := parseOptionalDuration(raw.ScrapeInterval)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s.scrapeInterval: %w", path, err))
	}
	out.ScrapeInterval = interval

	secretHeaders, headerErrs := resolveSecretHeaders(path, raw.Headers, raw.HeadersFromEnv, raw.HeadersFromFile, lookup)
	errs = append(errs, headerErrs...)
	out.SecretHeaders = secretHeaders

	hasBasic := raw.BasicAuth != nil
	hasBearer := raw.BearerTokenEnv != "" || raw.BearerTokenFile != ""
	if hasBasic && hasBearer {
		errs = append(errs, fmt.Errorf("%s: basicAuth and bearerToken are mutually exclusive", path))
	}
	if hasBasic {
		if raw.BasicAuth.Username == "" {
			errs = append(errs, fmt.Errorf("%s.basicAuth.username: required", path))
		}
		out.BasicAuthUser = raw.BasicAuth.Username
		secret, secretErrs := resolveSecret(path+".basicAuth", "password", raw.BasicAuth.PasswordEnv, raw.BasicAuth.PasswordFile, true, lookup)
		errs = append(errs, secretErrs...)
		out.BasicAuthPassword = secret
	}
	if hasBearer {
		secret, secretErrs := resolveSecret(path, "bearerToken", raw.BearerTokenEnv, raw.BearerTokenFile, false, lookup)
		errs = append(errs, secretErrs...)
		out.BearerToken = secret
	}
	return out, errs
}

func convertClickHouse(path string, raw fileClickHouse, lookup func(string) (string, bool)) (ClickHouseDatasource, []error) {
	var errs []error
	if raw.Addr == "" {
		errs = append(errs, fmt.Errorf("%s.addr: required", path))
	}
	if raw.Username == "" {
		errs = append(errs, fmt.Errorf("%s.username: required", path))
	}
	secret, secretErrs := resolveSecret(path, "password", raw.PasswordEnv, raw.PasswordFile, true, lookup)
	errs = append(errs, secretErrs...)
	return ClickHouseDatasource{
		Addr:     raw.Addr,
		Database: raw.Database,
		Username: raw.Username,
		Password: secret,
		TLS:      raw.TLS,
	}, errs
}

func resolveSecret(path, name, envName, file string, required bool, lookup func(string) (string, bool)) (datasource.Secret, []error) {
	envField, fileField := name+"Env", name+"File"
	switch {
	case envName != "" && file != "":
		return datasource.Secret{}, []error{fmt.Errorf("%s: %s and %s are mutually exclusive", path, envField, fileField)}
	case envName == "" && file == "":
		if required {
			return datasource.Secret{}, []error{fmt.Errorf("%s: one of %s or %s is required", path, envField, fileField)}
		}
		return datasource.Secret{}, nil
	case envName != "":
		return secretFromEnv(path+"."+envField, envName, lookup)
	default:
		return secretFromFile(path+"."+fileField, file)
	}
}

func secretFromEnv(path, envName string, lookup func(string) (string, bool)) (datasource.Secret, []error) {
	if !strings.HasPrefix(envName, secretEnvPrefix) {
		return datasource.Secret{}, []error{fmt.Errorf("%s: must start with %s", path, secretEnvPrefix)}
	}
	if slices.Contains(serverEnvNames, envName) {
		return datasource.Secret{}, []error{fmt.Errorf("%s: %s is reserved for the server's own configuration", path, envName)}
	}
	v, _ := lookup(envName)
	if v == "" {
		return datasource.Secret{}, []error{fmt.Errorf("%s: %s is not set or empty", path, envName)}
	}
	return datasource.NewSecret(v), nil
}

func secretFromFile(path, file string) (datasource.Secret, []error) {
	b, err := os.ReadFile(file) //nolint:gosec // the path is operator configuration
	if err != nil {
		return datasource.Secret{}, []error{fmt.Errorf("%s: %w", path, err)}
	}
	v := strings.TrimRight(string(b), "\r\n")
	if v == "" {
		return datasource.Secret{}, []error{fmt.Errorf("%s: %s is empty", path, file)}
	}
	return datasource.NewSecret(v), nil
}

func resolveSecretHeaders(path string, plain, fromEnv, fromFile map[string]string, lookup func(string) (string, bool)) (map[string]datasource.Secret, []error) {
	var errs []error
	seen := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(plain)) {
		canonical := http.CanonicalHeaderKey(name)
		if seen[canonical] {
			errs = append(errs, fmt.Errorf("%s.headers[%s]: header %q is set more than once", path, name, canonical))
		}
		seen[canonical] = true
	}
	out := map[string]datasource.Secret{}
	resolve := func(field string, source map[string]string, read func(string, string) (datasource.Secret, []error)) {
		for _, name := range slices.Sorted(maps.Keys(source)) {
			entryPath := fmt.Sprintf("%s.%s[%s]", path, field, name)
			canonical := http.CanonicalHeaderKey(name)
			if seen[canonical] {
				errs = append(errs, fmt.Errorf("%s: header %q is set more than once", entryPath, canonical))
				continue
			}
			seen[canonical] = true
			secret, secretErrs := read(entryPath, source[name])
			errs = append(errs, secretErrs...)
			out[name] = secret
		}
	}
	resolve("headersFromEnv", fromEnv, func(p, v string) (datasource.Secret, []error) { return secretFromEnv(p, v, lookup) })
	resolve("headersFromFile", fromFile, secretFromFile)
	if len(out) == 0 {
		return nil, errs
	}
	return out, errs
}

func parseOptionalDuration(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, errors.New("invalid duration")
	}
	if d < 0 {
		return 0, errors.New("must not be negative")
	}
	return d, nil
}
