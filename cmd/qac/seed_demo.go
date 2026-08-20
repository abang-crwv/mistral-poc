package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"

	"qac/internal/store"
)

func seedDemoCmd() *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "seed-demo",
		Short: "Seed one demo run",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDB, err := resolveDBPath(dbPath)
			if err != nil {
				return err
			}

			ctx := context.Background()
			s, err := store.Open(ctx, resolvedDB)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()

			id := ulid.MustNew(ulid.Timestamp(time.Now()), ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)).String()
			payload, err := json.Marshal(map[string]any{
				"template_id": "firmware-release-canary",
				"inputs": map[string]any{
					"bundle_tag":    "gb200-fw-2026-05-canary-demo",
					"canary_racks":  []string{"dh3-r012-us-east-01a"},
					"rlcc_workflow": "gb200-rack-bringup-v4",
				},
				"created_by": "wpena",
			})
			if err != nil {
				return fmt.Errorf("marshal payload: %w", err)
			}
			if err := s.AppendEvent(ctx, id, "RunCreated", payload); err != nil {
				return fmt.Errorf("append event: %w", err)
			}
			fmt.Printf("Seeded run %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite path (default: $XDG_DATA_HOME/qac/qac.db)")
	return cmd
}
