package grafanaconn

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/pkg/grafana"
)

const internalMessage = "Internal error."

func describe(err error) string {
	if message, ok := describeFetch(err); ok {
		return message
	}
	var input *grafana.InputError
	var unresolved *imports.UnresolvedError
	var invalid *dashboards.InvalidSpecError
	switch {
	case errors.As(err, &input):
		return input.Message
	case errors.As(err, &unresolved):
		return fmt.Sprintf("%d datasources have no match; map them and import again.", unmatched(unresolved))
	case errors.As(err, &invalid):
		return fmt.Sprintf("The dashboard has %d issues that block saving it; later versions run every query, so the newer Grafana version was not saved.", len(invalid.Issues))
	case errors.Is(err, folders.ErrForbidden), errors.Is(err, folders.ErrNoAccess):
		return "You cannot save this dashboard in its folder."
	case errors.Is(err, dashboards.ErrNotFound), errors.Is(err, dashboards.ErrFolderNotFound), errors.Is(err, imports.ErrNotFound):
		return "The dashboard or its folder was deleted during the import."
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "The import stopped before this dashboard."
	}
	return internalMessage
}

func unmatched(e *imports.UnresolvedError) int {
	n := 0
	for _, d := range e.Datasources {
		if d.Unmatched() {
			n++
		}
	}
	return n
}

func describeFetch(err error) (string, bool) {
	var hostErr *egress.HostError
	host := "Grafana"
	if errors.As(err, &hostErr) {
		host = hostErr.Host
	}
	var status *egress.StatusError
	switch {
	case errors.As(err, &status) && (status.Status == http.StatusUnauthorized || status.Status == http.StatusForbidden):
		return fmt.Sprintf("Grafana refused the token (status %d).", status.Status), true
	case errors.As(err, &status) && status.Status == http.StatusNotFound:
		return "Grafana answered 404; check the URL and that the dashboard still exists.", true
	case errors.As(err, &status):
		return fmt.Sprintf("Grafana answered with status %d.", status.Status), true
	case errors.Is(err, egress.ErrNotAllowed):
		return host + " is not on the outbound allowlist.", true
	case errors.Is(err, egress.ErrInvalidURL):
		return "The connection URL is not an address this server may fetch.", true
	case errors.Is(err, egress.ErrRedirected):
		return "Grafana answered with a redirect, which is not followed; check the URL.", true
	case errors.Is(err, egress.ErrTooLarge):
		return "Grafana's answer is larger than 8 MiB.", true
	case errors.Is(err, egress.ErrContentType), errors.Is(err, errBadResponse):
		return "Grafana did not answer with the JSON its HTTP API returns; check the URL.", true
	case errors.Is(err, egress.ErrBlockedAddress):
		return host + " points to an address this server may not reach.", true
	case errors.Is(err, egress.ErrUnavailable):
		return host + " could not be reached.", true
	}
	return "", false
}
