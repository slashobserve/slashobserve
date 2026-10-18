package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	envServer = "OBSERVE_SERVER"
	envToken  = "OBSERVE_TOKEN"

	errNoServer = "no server configured: pass --server, set " + envServer + ", or run observe login --server URL"
	msgSignIn   = "not signed in, or the key is invalid; run observe login"
)

type fileConfig struct {
	Server string
	Token  string

	extra map[string]json.RawMessage
}

type settings struct {
	server string
	token  string
}

func configPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" || !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find the config directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "slashobserve", "cli.json"), nil
}

func loadFileConfig(path string) (fileConfig, error) {
	var cfg fileConfig
	data, err := os.ReadFile(path) //nolint:gosec // the path is the user's CLI config location
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	fields, err := decodeFields(data)
	if err != nil {
		return fileConfig{}, fmt.Errorf("%s is not a valid CLI config (fix or delete it): %w", path, err)
	}
	for _, field := range fields {
		var target *string
		switch {
		case strings.EqualFold(field.name, "server"):
			target = &cfg.Server
		case strings.EqualFold(field.name, "token"):
			target = &cfg.Token
		default:
			if cfg.extra == nil {
				cfg.extra = make(map[string]json.RawMessage)
			}
			cfg.extra[field.name] = field.value
			continue
		}
		*target = ""
		if err := json.Unmarshal(field.value, target); err != nil {
			return fileConfig{}, fmt.Errorf("%s is not a valid CLI config (fix or delete it): field %q: %w", path, field.name, err)
		}
	}
	return cfg, nil
}

type rawField struct {
	name  string
	value json.RawMessage
}

func decodeFields(data []byte) ([]rawField, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, expectEnd(decoder)
	}
	if delim, ok := first.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("the top level must be an object")
	}
	var fields []rawField
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		name, _ := key.(string)
		fields = append(fields, rawField{name: name, value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return fields, expectEnd(decoder)
}

func expectEnd(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the config object")
	}
	return nil
}

func saveFileConfig(path string, cfg fileConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory needs the owner execute bit
			return fmt.Errorf("restrict %s: %w", dir, err)
		}
	}
	fields := make(map[string]json.RawMessage, len(cfg.extra)+2)
	for name, raw := range cfg.extra {
		fields[name] = raw
	}
	for name, value := range map[string]string{"server": cfg.Server, "token": cfg.Token} {
		if value == "" {
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode the config: %w", err)
		}
		fields[name] = raw
	}
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".cli-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	handle, err := os.Open(dir) //nolint:gosec // the directory is the user's CLI config location
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}

func (a *app) resolve() (settings, error) {
	resolved := settings{server: a.server, token: a.token}
	if resolved.server == "" {
		resolved.server = os.Getenv(envServer)
	}
	if resolved.token == "" {
		resolved.token = os.Getenv(envToken)
	}
	if resolved.server == "" || resolved.token == "" {
		path, err := configPath()
		if err != nil {
			return settings{}, err
		}
		saved, err := loadFileConfig(path)
		if err != nil {
			return settings{}, err
		}
		if resolved.server == "" {
			resolved.server = saved.Server
		}
		if resolved.token == "" && saved.Token != "" {
			if err := savedKeyApplies(resolved.server, saved.Server); err != nil {
				return settings{}, err
			}
			resolved.token = saved.Token
		}
	}
	resolved.token = strings.TrimSpace(resolved.token)
	return resolved, nil
}

func savedKeyApplies(server, savedServer string) error {
	current, err := normalizeServer(server)
	if err != nil {
		return err
	}
	owner := "another server"
	saved, err := normalizeServer(savedServer)
	if err == nil {
		if sameServer(current, saved) {
			return nil
		}
		owner = saved.String()
	}
	return fmt.Errorf("the saved key belongs to %s; pass --token or run observe login --server %s", owner, current.String())
}

func sameServer(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host) && a.Path == b.Path
}

func normalizeServer(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New(errNoServer)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" {
		return nil, errors.New("the server must be an absolute http or https URL such as https://observe.example.com")
	}
	if u.User != nil {
		return nil, errors.New("the server URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, errors.New("the server URL must not contain a query or fragment")
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return nil, fmt.Errorf("refusing plain http to %s: use https, or a loopback address for local stacks", u.Hostname())
	}
	dropDefaultPort(u)
	u.Path = strings.TrimRight(cleanPath(u.Path), "/")
	u.RawPath = ""
	return u, nil
}

func dropDefaultPort(u *url.URL) {
	if (u.Scheme != "https" || u.Port() != "443") && (u.Scheme != "http" || u.Port() != "80") {
		return
	}
	host := u.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
}

func cleanPath(p string) string {
	if p == "" {
		return p
	}
	return path.Clean(p)
}

func isLoopback(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func validToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}
