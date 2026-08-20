// Package qac hosts //go:embed declarations for the SPA bundle and
// canary templates. The embed patterns are relative to this file's
// directory, so the file must live at the module root.
package qac

import "embed"

//go:embed all:web/dist
var DistFS embed.FS

//go:embed templates/*.yaml
var TemplatesFS embed.FS
