package main

import (
	"strings"
	"testing"
)

func TestCheckTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		remote  string
		wantErr string
	}{
		{name: "localhost on the replicas port", target: "http://localhost:48080"},
		{name: "ipv4 loopback on the replicas port", target: "http://127.0.0.1:48080"},
		{name: "ipv6 loopback on the replicas port", target: "http://[::1]:48080"},
		{name: "loopback without a port", target: "http://localhost"},
		{name: "demo port on ipv4 loopback", target: "http://127.0.0.1:8080", wantErr: "8080 is the demo stack; run make replicas-up and target http://localhost:48080"},
		{name: "demo port on localhost", target: "http://localhost:8080", wantErr: "8080 is the demo stack"},
		{name: "demo port on ipv6 loopback", target: "http://[::1]:8080", wantErr: "8080 is the demo stack"},
		{name: "remote host without --remote", target: "https://so.example.com", wantErr: "--remote so.example.com"},
		{name: "remote host named by --remote", target: "https://so.example.com", remote: "so.example.com"},
		{name: "remote host with another --remote", target: "https://so.example.com", remote: "other.example.com", wantErr: "--remote so.example.com"},
		{name: "remote host on port 8080 is not the demo", target: "https://so.example.com:8080", remote: "so.example.com"},
		{name: "lookalike of localhost", target: "http://localhost.example.com:48080", wantErr: "--remote localhost.example.com"},
		{name: "ftp", target: "ftp://localhost", wantErr: "http:// or https://"},
		{name: "no host", target: "http://", wantErr: "names no host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := checkTarget(tt.target, tt.remote)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkTarget(%q, %q) = %v, want nil", tt.target, tt.remote, err)
				}
				if u == nil || u.Host == "" {
					t.Fatalf("checkTarget(%q, %q) returned no URL", tt.target, tt.remote)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkTarget(%q, %q) = %v, want an error containing %q", tt.target, tt.remote, err, tt.wantErr)
			}
		})
	}
}
