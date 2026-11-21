package config

import (
	"strings"
	"testing"
)

func TestAuditExportIsOffWithoutAnEndpoint(t *testing.T) {
	cfg, err := Load(envOf(llmEnv(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuditExport.Endpoint != nil || cfg.AuditExport.Headers != nil {
		t.Fatalf("export configured with nothing set: %+v", cfg.AuditExport)
	}
}

func TestAuditExportReadsTheEndpointAndTheHeadersFile(t *testing.T) {
	file := writeFile(t, "headers", "Authorization: Bearer abc\n\n  x-tenant :  one  \n")
	cfg, err := Load(envOf(llmEnv(map[string]string{
		envAuditOTLPEndpoint: "http://collector:4318/base",
		envAuditOTLPHeaders:  file,
	})))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuditExport.Endpoint.String(); got != "http://collector:4318/base" {
		t.Fatalf("endpoint = %q", got)
	}
	want := map[string]string{"Authorization": "Bearer abc", "X-Tenant": "one"}
	if len(cfg.AuditExport.Headers) != len(want) {
		t.Fatalf("headers = %d, want %d", len(cfg.AuditExport.Headers), len(want))
	}
	for name, value := range want {
		secret, ok := cfg.AuditExport.Headers[name]
		if !ok || secret.Reveal() != value {
			t.Fatalf("header %q missing or wrong", name)
		}
	}
}

func TestAuditExportRefusesBadSettingsByName(t *testing.T) {
	tests := []struct {
		name string
		env  func(t *testing.T) map[string]string
		want string
	}{
		{"headers without an endpoint", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPHeaders: writeFile(t, "h", "A: b\n")}
		}, envAuditOTLPHeaders + " requires " + envAuditOTLPEndpoint},
		{"a relative endpoint", func(*testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "collector:4318"}
		}, envAuditOTLPEndpoint},
		{"a non http scheme", func(*testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "grpc://collector:4317"}
		}, envAuditOTLPEndpoint},
		{"a missing headers file", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: t.TempDir() + "/absent"}
		}, "cannot read the file"},
		{"an empty headers file", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: writeFile(t, "h", "\n\n")}
		}, "the file has no header"},
		{"a line without a colon", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: writeFile(t, "h", "A: b\nbroken\n")}
		}, "line 2"},
		{"a bad header name", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: writeFile(t, "h", "bad name: v\n")}
		}, "line 1"},
		{"an empty value", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: writeFile(t, "h", "A:\n")}
		}, `header "A"`},
		{"a control character in a value", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: writeFile(t, "h", "A: b\x01c\n")}
		}, `header "A"`},
		{"the same header twice", func(t *testing.T) map[string]string {
			return map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: writeFile(t, "h", "A: 1\na: 2\n")}
		}, "already set on line 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(tt.env(t))))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestAuditExportErrorsNeverHoldTheHeaderValues(t *testing.T) {
	file := writeFile(t, "h", "Authorization: top-secret-token\nAuthorization: other-secret\n")
	_, err := Load(envOf(llmEnv(map[string]string{envAuditOTLPEndpoint: "http://c:4318", envAuditOTLPHeaders: file})))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "top-secret-token") || strings.Contains(err.Error(), "other-secret") {
		t.Fatalf("error leaks a header value: %v", err)
	}
}
