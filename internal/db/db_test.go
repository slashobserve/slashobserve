package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPoolConfigMaxConns(t *testing.T) {
	serviceFile := filepath.Join(t.TempDir(), "pg_service.conf")
	if err := os.WriteFile(serviceFile, []byte("[sized]\nhost=h\nuser=u\npool_max_conns=3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		dsn     string
		want    int32
		atLeast int32
	}{
		{name: "url with the parameter", dsn: "postgres://u@h:5432/d?sslmode=disable&pool_max_conns=3", want: 3},
		{name: "postgresql scheme with the parameter", dsn: "postgresql://u@h/d?pool_max_conns=40", want: 40},
		{name: "keyword with the parameter", dsn: "host=h user=u pool_max_conns=3", want: 3},
		{name: "keyword with the parameter first", dsn: "pool_max_conns = 3 host=h", want: 3},
		{name: "keyword with the parameter after a quoted value", dsn: "password='a b' pool_max_conns=3", want: 3},
		{name: "service file with the parameter", dsn: "service=sized servicefile=" + serviceFile, want: 3},
		{name: "url without the parameter", dsn: "postgres://u@h:5432/d?sslmode=disable", atLeast: minPoolConns},
		{name: "url whose parameter value contains the name", dsn: "postgres://u@h:5432/d?application_name=pool_max_conns", atLeast: minPoolConns},
		{name: "url whose path contains the name", dsn: "postgres://u@h/pool_max_conns", atLeast: minPoolConns},
		{name: "keyword without the parameter", dsn: "host=h user=u dbname=d", atLeast: minPoolConns},
		{name: "keyword quoted password contains the assignment", dsn: "host=h password='a pool_max_conns=3 b' user=u", atLeast: minPoolConns},
		{name: "keyword quoted password with an escaped quote", dsn: `host=h password='it\'s pool_max_conns=3' user=u`, atLeast: minPoolConns},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := poolConfig(tt.dsn)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want != 0 && cfg.MaxConns != tt.want {
				t.Fatalf("MaxConns = %d, want %d", cfg.MaxConns, tt.want)
			}
			if cfg.MaxConns < tt.atLeast {
				t.Fatalf("MaxConns = %d, want at least %d", cfg.MaxConns, tt.atLeast)
			}
		})
	}
}
