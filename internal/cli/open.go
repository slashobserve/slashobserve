package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

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
