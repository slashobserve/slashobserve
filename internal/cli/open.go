package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/links"
)

var grafanaUIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func (a *app) addOpenFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&a.open, "open", false, "open the dashboard in the browser once it is saved")
}

func (a *app) openLink(out *output, address string) {
	if !a.open {
		return
	}
	if err := openInBrowser(runtime.GOOS, address); err != nil {
		out.warn("could not open the browser: %s", text(err.Error()))
	}
}

func openInBrowser(goos, address string) error {
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("the address is not an http(s) URL")
	}
	name, args := opener(goos, parsed.String())
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s was not found", name)
	}
	if err := exec.Command(path, args...).Run(); err != nil { //nolint:gosec // a fixed opener per platform, given one checked http(s) URL
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func opener(goos, address string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{address}
	case "windows":
		// cmd's start would parse & and | in a server-supplied address as shell syntax, so the URL handler gets it as one argument.
		return "rundll32", []string{"url.dll,FileProtocolHandler", address}
	default:
		return "xdg-open", []string{address}
	}
}

func (a *app) openCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open [<dashboard id, Grafana uid or address>]",
		Short: "Open the server, a dashboard or an address on it in the browser",
		Long:  "open prints the address and opens it in the browser.\nWith no argument it opens the server; a dashboard id or the uid of an imported Grafana dashboard opens that dashboard; an address opens only when it is on the server.",
		Example: "  observe open\n" +
			"  observe open <dashboard-id>\n" +
			"  observe open checkout-overview",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("%s takes at most one dashboard id, Grafana uid or address", cmd.CommandPath())
			}
			return nil
		},
		RunE: a.runOpen,
	}
}

func (a *app) serverAddress() (*url.URL, error) {
	server := cmp.Or(a.server, os.Getenv(envServer))
	if server == "" {
		resolved, err := a.resolve()
		if err != nil {
			return nil, err
		}
		server = resolved.server
	}
	return normalizeServer(server)
}

func (a *app) runOpen(_ *cobra.Command, args []string) error {
	base, err := a.serverAddress()
	if err != nil {
		return err
	}
	target := ""
	if len(args) == 1 {
		target = strings.TrimSpace(args[0])
	}
	address, err := openTarget(base, target)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		data, err := json.Marshal(map[string]string{"url": address})
		if err != nil {
			return err
		}
		out.body(data)
	} else {
		out.line("%s", address)
	}
	if err := openInBrowser(runtime.GOOS, address); err != nil {
		return fmt.Errorf("could not open the browser: %s", text(err.Error()))
	}
	return nil
}

func openTarget(base *url.URL, target string) (string, error) {
	switch {
	case target == "":
		return base.String(), nil
	case idPattern.MatchString(target), grafanaUIDPattern.MatchString(target):
		return links.Dashboard(base, target, ""), nil
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", usagef("%q is not a dashboard id, a Grafana uid or an http(s) address", text(target))
	}
	dropDefaultPort(parsed)
	if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
		return "", usagef("%s is not on %s", text(target), base.String())
	}
	return parsed.String(), nil
}
