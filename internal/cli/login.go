package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const maxKeyBytes = 4096

type principal struct {
	Name string `json:"name"`
}

func (a *app) loginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save a service-account key and the server it belongs to",
		Long:  "login checks a service-account key against the server and saves both to the CLI config file.\nCreate a key on the server's API keys page (<server>/keys).",
		Args:  noArgs,
		RunE:  a.runLogin,
	}
	cmd.Flags().BoolVar(&a.withToken, "with-token", false, "read the key from standard input instead of prompting")
	return cmd
}

func (a *app) runLogin(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Changed("token") {
		return usagef("observe login does not take --token: pipe the key in with --with-token, or omit it to be prompted")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	saved, err := loadFileConfig(path)
	if err != nil {
		return err
	}
	rawServer := a.server
	if rawServer == "" {
		rawServer = os.Getenv(envServer)
	}
	if rawServer == "" {
		rawServer = saved.Server
	}
	base, err := normalizeServer(rawServer)
	if err != nil {
		return err
	}
	a.hintKeysPage(base)
	key, err := a.readKey(cmd.Context())
	if err != nil {
		return err
	}
	c, err := a.clientFor(base, key)
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "me", nil)
	if errors.Is(err, errUnauthorized) {
		return errors.New("the server did not accept that key")
	}
	if err != nil {
		return err
	}
	who, err := decode[principal](data)
	if err != nil {
		return err
	}
	saved.Server, saved.Token = base.String(), key
	if err := saveFileConfig(path, saved); err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	out.confirm("Signed in to %s as %s", base.String(), text(who.Name))
	return nil
}

func (a *app) hintKeysPage(base *url.URL) {
	if _, interactive := a.interactiveInput(); !interactive || a.withToken || a.json {
		return
	}
	_, _ = fmt.Fprintf(a.std.Err, "Create a key at %s\n", base.JoinPath("settings", "keys").String())
}

func (a *app) interactiveInput() (*os.File, bool) {
	file, isFile := a.std.In.(*os.File)
	return file, isFile && term.IsTerminal(int(file.Fd()))
}

func (a *app) readKey(ctx context.Context) (string, error) {
	file, interactive := a.interactiveInput()
	if a.withToken {
		read := func() ([]byte, error) { return io.ReadAll(io.LimitReader(a.std.In, maxKeyBytes+1)) }
		data, err := readInterruptible(ctx, a.std.Err, file, interactive, read)
		if errors.Is(err, errInterrupted) {
			return "", err
		}
		if err != nil {
			return "", fmt.Errorf("read the key from standard input: %w", err)
		}
		return checkKey(string(data))
	}
	if !interactive {
		return "", usagef("standard input is not a terminal: pipe the key in and pass --with-token")
	}
	_, _ = fmt.Fprint(a.std.Err, "Service account key: ")
	data, err := readInterruptible(ctx, a.std.Err, file, true, func() ([]byte, error) { return term.ReadPassword(int(file.Fd())) })
	if err != nil {
		if errors.Is(err, errInterrupted) {
			return "", err
		}
		_, _ = fmt.Fprintln(a.std.Err)
		return "", fmt.Errorf("read the key: %w", err)
	}
	_, _ = fmt.Fprintln(a.std.Err)
	return checkKey(string(data))
}

var errInterrupted = errors.New("interrupted")

func readInterruptible(ctx context.Context, notice io.Writer, file *os.File, interactive bool, read func() ([]byte, error)) ([]byte, error) {
	var saved *term.State
	if interactive {
		saved, _ = term.GetState(int(file.Fd()))
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := read()
		done <- result{data, err}
	}()
	select {
	case r := <-done:
		return r.data, r.err
	case <-ctx.Done():
		if saved != nil {
			_ = term.Restore(int(file.Fd()), saved)
		}
		_, _ = fmt.Fprintln(notice)
		return nil, errInterrupted
	}
}

func checkKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	switch {
	case key == "":
		return "", usagef("no key given")
	case len(key) > maxKeyBytes:
		return "", usagef("the key is too long")
	case !validToken(key):
		return "", usagef("the key contains characters a key never has")
	}
	return key, nil
}

func (a *app) logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved key and keep the saved server",
		Args:  noArgs,
		RunE:  a.runLogout,
	}
}

func (a *app) runLogout(_ *cobra.Command, _ []string) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	saved, err := loadFileConfig(path)
	if err != nil {
		return err
	}
	out := a.output()
	if saved.Token == "" {
		out.confirm("No saved key")
	} else {
		saved.Token = ""
		if err := saveFileConfig(path, saved); err != nil {
			return err
		}
		out.confirm("Signed out; the key stays valid until it is revoked on the server")
	}
	if os.Getenv(envToken) != "" {
		out.notice("%s is still set in the environment", envToken)
	}
	return nil
}
