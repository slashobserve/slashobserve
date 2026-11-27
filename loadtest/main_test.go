package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseBuilds(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []int
		wantErr bool
	}{
		{name: "one size", in: "20", want: []int{20}},
		{name: "several sizes", in: "20,50", want: []int{20, 50}},
		{name: "spaces around a size", in: "2, 16", want: []int{2, 16}},
		{name: "empty", in: "", wantErr: true},
		{name: "zero", in: "0", wantErr: true},
		{name: "negative", in: "20,-1", wantErr: true},
		{name: "not a number", in: "twenty", wantErr: true},
		{name: "trailing comma", in: "20,", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBuilds(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseBuilds(%q) = %v, want an error", tt.in, got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseBuilds(%q) = %v, %v, want %v", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestParseScenarios(t *testing.T) {
	all := []string{"viewers", "streams", "mcp", "agent"}
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr string
	}{
		{name: "one", in: []string{"mcp"}, want: []string{"mcp"}},
		{name: "several keep their order", in: []string{"agent", "viewers"}, want: []string{"agent", "viewers"}},
		{name: "all", in: []string{"all"}, want: all},
		{name: "all after another", in: []string{"mcp", "all"}, want: all},
		{name: "none", in: nil, wantErr: "name a scenario"},
		{name: "unknown", in: []string{"viewers", "soak"}, wantErr: `unknown scenario "soak"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScenarios(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseScenarios(%v) = %v, %v, want an error containing %q", tt.in, got, err, tt.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseScenarios(%v) = %v, %v, want %v", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestResolveRefusesAReplicaThatIsNotLoopback(t *testing.T) {
	keys := filepath.Join(t.TempDir(), "keys")
	if err := os.WriteFile(keys, []byte("sok_abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		replicas []string
		remote   string
		wantErr  string
	}{
		{name: "loopback replicas", replicas: []string{"http://localhost:48081", "http://127.0.0.1:48082"}},
		{name: "no replicas"},
		{name: "a remote replica", replicas: []string{"http://localhost:48081", "https://other.example.com"}, wantErr: "--replica: other.example.com is not this computer"},
		{name: "a remote replica named by --remote", replicas: []string{"https://other.example.com"}, remote: "other.example.com"},
		{name: "the demo stack as a replica", replicas: []string{"http://localhost:58080"}, wantErr: "--replica: 58080 is the demo stack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config{label: "run-1", remote: tt.remote, replicas: tt.replicas}
			target := "http://localhost:48080"
			if tt.remote != "" {
				target = "https://" + tt.remote
			}
			err := resolve(cfg, target, keys, keys, "20", []string{"viewers"})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("resolve = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("resolve = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
