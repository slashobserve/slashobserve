package server

import (
	"net/http"

	"github.com/klauspost/compress/gzhttp"
)

const compressMinBytes = 1024

func compressReads(next http.Handler) (http.Handler, error) {
	wrap, err := gzhttp.NewWrapper(
		gzhttp.MinSize(compressMinBytes),
		gzhttp.EnableZstd(false),
		gzhttp.ExceptContentTypes([]string{"text/event-stream"}),
	)
	if err != nil {
		return nil, err
	}
	gzipped := wrap(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if breachSafeMethod(r.Method) {
			gzipped.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}

func breachSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}
