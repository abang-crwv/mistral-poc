package api

import (
	"net/http"
	"runtime/debug"
)

func healthHandler(dbPath string) http.HandlerFunc {
	version := readVersion()
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"version": version,
			"db_path": dbPath,
		})
	}
}

// readVersion returns the build version from BuildInfo. Falls back to
// "dev" when running via `go run` or `go test`, which don't embed VCS info.
func readVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			n := 7
			if len(s.Value) < n {
				n = len(s.Value)
			}
			return s.Value[:n]
		}
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
