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
	"github.com/goccy/go-yaml/ast"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	secretEnvPrefix = "SLASHOBSERVE_"

	overflowSuffix = "( overflow )"

	typePrometheus    = "prometheus"
	typeClickHouse    = "clickhouse"
	typeLoki          = "loki"
	typeTempo         = "tempo"
	typeElasticsearch = "elasticsearch"
	typeOpenSearch    = "opensearch"

	maxGrafanaUIDs     = 20
	maxGrafanaUIDBytes = 100

	maxIndexBytes     = 1024
	maxFieldNameBytes = 256
	allIndices        = "_all"
)

var queryLanguages = []string{"kql", "lucene"}

type Datasource struct {
	UID, Type, Name string
	GrafanaUIDs     []string
	Prometheus      *PrometheusDatasource
	ClickHouse      *ClickHouseDatasource
	Loki            *LokiDatasource
	Tempo           *TempoDatasource
	Elasticsearch   *SearchDatasource
	OpenSearch      *SearchDatasource
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

type HTTPConnection struct {
	URL               string
	Headers           map[string]string
	SecretHeaders     map[string]datasource.Secret
	BasicAuthUser     string
	BasicAuthPassword datasource.Secret
	BearerToken       datasource.Secret
}

type LokiDatasource struct {
	HTTPConnection
	TenantID string
}

type TempoDatasource struct {
	HTTPConnection
}

type SearchDatasource struct {
	HTTPConnection
	APIKey        datasource.Secret
	Index         string
	TimeField     string
	QueryLanguage string
	Fields        FieldRoles
}

type FieldRoles struct {
	Message, Level, Service, TraceID, SpanID string
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
	UID           yamlString      `yaml:"uid"`
	Type          string          `yaml:"type"`
	Name          yamlString      `yaml:"name"`
	GrafanaUIDs   []yamlString    `yaml:"grafanaUids"`
	Prometheus    *filePrometheus `yaml:"prometheus"`
	ClickHouse    *fileClickHouse `yaml:"clickhouse"`
	Loki          *fileLoki       `yaml:"loki"`
	Tempo         *fileHTTP       `yaml:"tempo"`
	Elasticsearch *fileSearch     `yaml:"elasticsearch"`
	OpenSearch    *fileSearch     `yaml:"opensearch"`
	Limits        fileLimits      `yaml:"limits"`
}

type yamlString struct {
	value   string
	problem string
}

func (s *yamlString) UnmarshalYAML(node ast.Node) error {
	switch n := node.(type) {
	case *ast.TagNode:
		s.problem = "must not use a YAML tag; quote the value instead"
	case *ast.NullNode:
	case *ast.StringNode:
		s.value = n.Value
	case *ast.LiteralNode:
		s.value = n.Value.Value
	case *ast.MappingNode, *ast.MappingValueNode, *ast.SequenceNode:
		s.problem = "must be a string"
	default:
		s.problem = "must be a string (quote it)"
	}
	return nil
}

type filePrometheus struct {
	HTTP           fileHTTP `yaml:",inline"`
	ScrapeInterval string   `yaml:"scrapeInterval"`
}

type fileLoki struct {
	HTTP     fileHTTP `yaml:",inline"`
	TenantID string   `yaml:"tenantId"`
}

type fileSearch struct {
	HTTP          fileHTTP       `yaml:",inline"`
	Index         string         `yaml:"index"`
	TimeField     string         `yaml:"timeField"`
	QueryLanguage string         `yaml:"queryLanguage"`
	Fields        fileFieldRoles `yaml:"fields"`
	APIKeyEnv     string         `yaml:"apiKeyEnv"`
	APIKeyFile    string         `yaml:"apiKeyFile"`
}

type fileFieldRoles struct {
	Message string `yaml:"message"`
	Level   string `yaml:"level"`
	Service string `yaml:"service"`
	TraceID string `yaml:"traceId"`
	SpanID  string `yaml:"spanId"`
}

type fileHTTP struct {
	URL             string            `yaml:"url"`
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
	uids := indexUIDs(*doc.Datasources)
	aliasOwners := map[string]int{}
	out := make([]Datasource, 0, len(*doc.Datasources))
	for i, raw := range *doc.Datasources {
		entryPath := fmt.Sprintf("datasources[%d]", i)
		ds, dsErrs := convertDatasource(entryPath, raw, lookup)
		aliases := convertGrafanaUIDs(entryPath, raw.GrafanaUIDs, &dsErrs)
		errs = append(errs, dsErrs...)
		if uid := raw.UID.value; datasource.ValidUID(uid) && uids.first[uid] != i {
			errs = append(errs, fmt.Errorf("%s.uid: duplicate %q", entryPath, uid))
		}
		for _, alias := range aliases {
			aliasPath := fmt.Sprintf("%s.grafanaUids[%d]", entryPath, alias.index)
			owner, listed := aliasOwners[alias.value]
			uidOwner, isUID := uids.first[alias.value]
			switch {
			case alias.value == raw.UID.value:
			case isUID:
				errs = append(errs, fmt.Errorf("%s: %q is the uid of datasources[%d]", aliasPath, alias.value, uidOwner))
				continue
			case listed:
				errs = append(errs, fmt.Errorf("%s: %q is already listed by datasource %s", aliasPath, alias.value, uids.label(owner)))
				continue
			}
			aliasOwners[alias.value] = i
			ds.GrafanaUIDs = append(ds.GrafanaUIDs, alias.value)
		}
		out = append(out, ds)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

type uidIndex struct {
	first  map[string]int
	count  map[string]int
	byItem []string
}

func indexUIDs(list []fileDatasource) uidIndex {
	idx := uidIndex{first: map[string]int{}, count: map[string]int{}, byItem: make([]string, len(list))}
	for i, raw := range list {
		idx.byItem[i] = raw.UID.value
		if uid := raw.UID.value; datasource.ValidUID(uid) {
			if _, taken := idx.first[uid]; !taken {
				idx.first[uid] = i
			}
			idx.count[uid]++
		}
	}
	return idx
}

func (idx uidIndex) label(i int) string {
	if uid := idx.byItem[i]; idx.count[uid] == 1 {
		return fmt.Sprintf("%q", uid)
	}
	return fmt.Sprintf("datasources[%d]", i)
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

	ds := Datasource{UID: raw.UID.value, Type: raw.Type, Name: raw.Name.value}
	switch {
	case raw.UID.problem != "":
		fail("uid", "%s", raw.UID.problem)
	case !datasource.ValidUID(raw.UID.value):
		fail("uid", "must match %s", datasource.UIDPattern)
	}

	if raw.Name.problem != "" {
		fail("name", "%s", raw.Name.problem)
	}

	limits, limitErrs := convertLimits(path+".limits", raw.Limits)
	errs = append(errs, limitErrs...)
	ds.Limits = limits

	present := map[string]bool{
		typePrometheus:    raw.Prometheus != nil,
		typeClickHouse:    raw.ClickHouse != nil,
		typeLoki:          raw.Loki != nil,
		typeTempo:         raw.Tempo != nil,
		typeElasticsearch: raw.Elasticsearch != nil,
		typeOpenSearch:    raw.OpenSearch != nil,
	}
	if _, known := present[raw.Type]; !known {
		fail("type", "must be %s, %s, %s, %s, %s or %s, got %q", typePrometheus, typeClickHouse, typeLoki, typeTempo, typeElasticsearch, typeOpenSearch, raw.Type)
		return ds, errs
	}
	for _, block := range slices.Sorted(maps.Keys(present)) {
		if block != raw.Type && present[block] {
			fail(block, "not allowed for type %s", raw.Type)
		}
	}
	if !present[raw.Type] {
		fail(raw.Type, "required for type %s", raw.Type)
		return ds, errs
	}

	block := path + "." + raw.Type
	var blockErrs []error
	switch raw.Type {
	case typePrometheus:
		p, pErrs := convertPrometheus(block, *raw.Prometheus, lookup)
		ds.Prometheus, blockErrs = &p, pErrs
	case typeClickHouse:
		c, cErrs := convertClickHouse(block, *raw.ClickHouse, lookup)
		ds.ClickHouse, blockErrs = &c, cErrs
	case typeLoki:
		conn, cErrs := convertHTTP(block, raw.Loki.HTTP, lookup)
		ds.Loki, blockErrs = &LokiDatasource{HTTPConnection: conn, TenantID: raw.Loki.TenantID}, cErrs
	case typeTempo:
		conn, cErrs := convertHTTP(block, *raw.Tempo, lookup)
		ds.Tempo, blockErrs = &TempoDatasource{HTTPConnection: conn}, cErrs
	case typeElasticsearch:
		s, sErrs := convertSearch(block, *raw.Elasticsearch, lookup)
		ds.Elasticsearch, blockErrs = &s, sErrs
	case typeOpenSearch:
		s, sErrs := convertSearch(block, *raw.OpenSearch, lookup)
		ds.OpenSearch, blockErrs = &s, sErrs
	}
	return ds, append(errs, blockErrs...)
}

func convertSearch(path string, raw fileSearch, lookup func(string) (string, bool)) (SearchDatasource, []error) {
	conn, errs := convertHTTP(path, raw.HTTP, lookup)
	out := SearchDatasource{
		HTTPConnection: conn,
		Index:          raw.Index,
		TimeField:      raw.TimeField,
		QueryLanguage:  raw.QueryLanguage,
		Fields:         FieldRoles{Message: raw.Fields.Message, Level: raw.Fields.Level, Service: raw.Fields.Service, TraceID: raw.Fields.TraceID, SpanID: raw.Fields.SpanID},
	}
	if problem := indexProblem(raw.Index); problem != "" {
		errs = append(errs, fmt.Errorf("%s.index: %s", path, problem))
	}
	for _, f := range []struct{ field, value string }{
		{"timeField", raw.TimeField},
		{"fields.message", raw.Fields.Message},
		{"fields.level", raw.Fields.Level},
		{"fields.service", raw.Fields.Service},
		{"fields.traceId", raw.Fields.TraceID},
		{"fields.spanId", raw.Fields.SpanID},
	} {
		if problem := fieldNameProblem(f.value); problem != "" {
			errs = append(errs, fmt.Errorf("%s.%s: %s", path, f.field, problem))
		}
	}
	if raw.QueryLanguage != "" && !slices.Contains(queryLanguages, raw.QueryLanguage) {
		errs = append(errs, fmt.Errorf("%s.queryLanguage: must be %s", path, strings.Join(queryLanguages, " or ")))
	}
	hasAPIKey := raw.APIKeyEnv != "" || raw.APIKeyFile != ""
	if hasAPIKey && raw.HTTP.BasicAuth != nil {
		errs = append(errs, fmt.Errorf("%s: apiKey and basicAuth are mutually exclusive", path))
	}
	if hasAPIKey && (raw.HTTP.BearerTokenEnv != "" || raw.HTTP.BearerTokenFile != "") {
		errs = append(errs, fmt.Errorf("%s: apiKey and bearerToken are mutually exclusive", path))
	}
	if hasAPIKey {
		secret, secretErrs := resolveSecret(path, "apiKey", raw.APIKeyEnv, raw.APIKeyFile, false, lookup)
		errs = append(errs, secretErrs...)
		out.APIKey = secret
	}
	return out, errs
}

func indexProblem(index string) string {
	switch {
	case index == "":
		return "required"
	case index == allIndices:
		return "index must name a pattern narrower than " + allIndices
	case len(index) > maxIndexBytes:
		return fmt.Sprintf("must be at most %d bytes", maxIndexBytes)
	case strings.ContainsFunc(index, unicode.IsSpace):
		return "must not contain whitespace"
	case index[0] == '-' || index[0] == '_':
		return "must not start with - or _"
	}
	return ""
}

func fieldNameProblem(name string) string {
	switch {
	case name == "":
		return ""
	case len(name) > maxFieldNameBytes:
		return fmt.Sprintf("must be at most %d bytes", maxFieldNameBytes)
	case strings.ContainsFunc(name, unicode.IsSpace):
		return "must not contain whitespace"
	}
	return ""
}

type grafanaAlias struct {
	index int
	value string
}

func convertGrafanaUIDs(path string, raw []yamlString, errs *[]error) []grafanaAlias {
	if len(raw) > maxGrafanaUIDs {
		*errs = append(*errs, fmt.Errorf("%s.grafanaUids: at most %d entries", path, maxGrafanaUIDs))
	}
	var out []grafanaAlias
	seen := map[string]bool{}
	for j, entry := range raw {
		alias := strings.TrimSpace(entry.value)
		problem := entry.problem
		if problem == "" {
			problem = grafanaUIDProblem(alias)
		}
		if problem != "" {
			*errs = append(*errs, fmt.Errorf("%s.grafanaUids[%d]: %s", path, j, problem))
			continue
		}
		if seen[alias] {
			*errs = append(*errs, fmt.Errorf("%s.grafanaUids[%d]: duplicate %q", path, j, alias))
			continue
		}
		seen[alias] = true
		out = append(out, grafanaAlias{index: j, value: alias})
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
	case strings.ContainsFunc(alias, isInvisible):
		return "must not contain invisible characters"
	}
	return ""
}

func isInvisible(r rune) bool { return unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) }

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
	conn, errs := convertHTTP(path, raw.HTTP, lookup)
	interval, err := parseOptionalDuration(raw.ScrapeInterval)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s.scrapeInterval: %w", path, err))
	}
	return PrometheusDatasource{
		URL:               conn.URL,
		ScrapeInterval:    interval,
		Headers:           conn.Headers,
		SecretHeaders:     conn.SecretHeaders,
		BasicAuthUser:     conn.BasicAuthUser,
		BasicAuthPassword: conn.BasicAuthPassword,
		BearerToken:       conn.BearerToken,
	}, errs
}

func convertHTTP(path string, raw fileHTTP, lookup func(string) (string, bool)) (HTTPConnection, []error) {
	var errs []error
	out := HTTPConnection{URL: raw.URL, Headers: raw.Headers}
	if raw.URL == "" {
		errs = append(errs, fmt.Errorf("%s.url: required", path))
	}

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
