package config

import (
	"strings"
	"testing"
)

func TestSessionRenewSetting(t *testing.T) {
	tests := []struct {
		name    string
		set     map[string]string
		want    int
		wantErr []string
	}{
		{name: "unset leaves the agent default", want: 0},
		{name: "default size", set: map[string]string{envAgentSessionRenewKiB: "256"}, want: 256 << 10},
		{name: "lowest", set: map[string]string{envAgentSessionRenewKiB: "32"}, want: 32 << 10},
		{name: "highest", set: map[string]string{envAgentSessionRenewKiB: "4096"}, want: 4096 << 10},
		{name: "below the lowest", set: map[string]string{envAgentSessionRenewKiB: "31"}, wantErr: []string{envAgentSessionRenewKiB + ": must be a number of KiB between 32 and 4096"}},
		{name: "above the highest", set: map[string]string{envAgentSessionRenewKiB: "4097"}, wantErr: []string{envAgentSessionRenewKiB + ": must be a number of KiB between 32 and 4096"}},
		{name: "negative", set: map[string]string{envAgentSessionRenewKiB: "-1"}, wantErr: []string{envAgentSessionRenewKiB}},
		{name: "not a number", set: map[string]string{envAgentSessionRenewKiB: "abc"}, wantErr: []string{envAgentSessionRenewKiB}},
		{name: "fraction", set: map[string]string{envAgentSessionRenewKiB: "1.5"}, wantErr: []string{envAgentSessionRenewKiB}},
		{
			name:    "reported next to an invalid run timeout",
			set:     map[string]string{envAgentSessionRenewKiB: "abc", envAgentRunTimeout: "soon"},
			wantErr: []string{envAgentSessionRenewKiB, envAgentRunTimeout},
		},
		{name: "valid next to a run timeout", set: map[string]string{envAgentSessionRenewKiB: "64", envAgentRunTimeout: "30m"}, want: 64 << 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(tt.set)))
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatal("Load succeeded, want an error")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("Load error = %v, want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Agent.SessionRenewBytes != tt.want {
				t.Fatalf("SessionRenewBytes = %d, want %d", cfg.Agent.SessionRenewBytes, tt.want)
			}
		})
	}
}

func TestAgentWorkersSetting(t *testing.T) {
	const message = envAgentWorkers + ": must be a whole number from 1 to 64"
	tests := []struct {
		name    string
		set     map[string]string
		want    int
		wantErr string
	}{
		{name: "unset gives the default", want: 8},
		{name: "lowest", set: map[string]string{envAgentWorkers: "1"}, want: 1},
		{name: "highest", set: map[string]string{envAgentWorkers: "64"}, want: 64},
		{name: "zero", set: map[string]string{envAgentWorkers: "0"}, wantErr: message},
		{name: "above the highest", set: map[string]string{envAgentWorkers: "65"}, wantErr: message},
		{name: "negative", set: map[string]string{envAgentWorkers: "-1"}, wantErr: message},
		{name: "fraction", set: map[string]string{envAgentWorkers: "2.5"}, wantErr: message},
		{name: "not a number", set: map[string]string{envAgentWorkers: "abc"}, wantErr: message},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(tt.set)))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Agent.Workers != tt.want {
				t.Fatalf("Workers = %d, want %d", cfg.Agent.Workers, tt.want)
			}
		})
	}
}
