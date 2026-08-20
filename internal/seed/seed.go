// Package seed loads embedded canary templates into the store at boot.
// Calling LoadEmbeddedTemplates is idempotent — the store uses an
// id-keyed upsert.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"qac/internal/store"
	"qac/internal/template"
)

// LoadEmbeddedTemplates walks fsys for *.yaml files, parses and
// validates each, and upserts them into s. The walk is rooted at "."
// so it works against both qac.TemplatesFS (an embed.FS) and any
// other fs.FS such as an fstest.MapFS used in tests.
func LoadEmbeddedTemplates(ctx context.Context, s *store.Store, fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		tpl, err := template.Parse(body)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if err := template.Validate(tpl); err != nil {
			return fmt.Errorf("validate %s: %w", path, err)
		}

		parsed, err := json.Marshal(tpl)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", path, err)
		}
		if err := s.UpsertTemplate(ctx, tpl.ID, tpl.Version, string(body), string(parsed)); err != nil {
			return fmt.Errorf("upsert %s: %w", path, err)
		}
		slog.Info("loaded embedded template", "id", tpl.ID, "version", tpl.Version, "path", path)
		return nil
	})
}
