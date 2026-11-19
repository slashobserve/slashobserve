package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (r *Registry) Handler() http.Handler {
	scrape := promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.RawPath != "" {
			http.NotFound(w, req)
			return
		}
		scrape.ServeHTTP(w, req)
	})
}
