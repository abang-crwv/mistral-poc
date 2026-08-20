package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"qac/internal/store"
	"qac/internal/template"
)

func templateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage canary templates",
	}
	cmd.AddCommand(templateLoadCmd())
	return cmd
}

func templateLoadCmd() *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "load <path>",
		Short: "Parse, validate, and upsert a template from a YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDB, err := resolveDBPath(dbPath)
			if err != nil {
				return err
			}
			body, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %s: %w", args[0], err)
			}
			tpl, err := template.Parse(body)
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}
			if err := template.Validate(tpl); err != nil {
				return fmt.Errorf("validate: %w", err)
			}
			parsed, err := json.Marshal(tpl)
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}

			ctx := context.Background()
			s, err := store.Open(ctx, resolvedDB)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()
			if err := s.UpsertTemplate(ctx, tpl.ID, tpl.Version, string(body), string(parsed)); err != nil {
				return fmt.Errorf("upsert: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "loaded %s v%d\n", tpl.ID, tpl.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite path (default: $XDG_DATA_HOME/qac/qac.db)")
	return cmd
}
