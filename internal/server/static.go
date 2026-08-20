package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// SPAHandler serves the SPA from the provided embed.FS rooted at
// "web/dist". The caller (cmd/qac) owns the embed directive because
// the path must be relative to its own package directory.
func SPAHandler(distFS embed.FS) (http.Handler, error) {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static file? Let the file server handle it.
		// Anything with a "." in the last path segment is treated as a file.
		if strings.Contains(lastSegment(r.URL.Path), ".") {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Otherwise serve index.html so the SPA router takes over.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}), nil
}

func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
