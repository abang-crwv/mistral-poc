package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"qac"
	"qac/internal/agent"
	"qac/internal/agent/canaryassessor"
	"qac/internal/alertcategoryclient"
	"qac/internal/alertclient"
	"qac/internal/awxclient"
	"qac/internal/engine"
	"qac/internal/firmwareclient"
	"qac/internal/flccclient"
	"qac/internal/gpuperfclient"
	"qac/internal/hpcverifclient"
	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
	"qac/internal/llmclient"
	"qac/internal/lokiclient"
	"qac/internal/probe"
	"qac/internal/probe/alertprobe"
	"qac/internal/probe/awxjobprobe"
	"qac/internal/probe/failcauseprobe"
	"qac/internal/probe/firmwareinventoryprobe"
	"qac/internal/probe/gpuperfprobe"
	"qac/internal/probe/hpcverifprobe"
	"qac/internal/probe/rlccactionprobe"
	"qac/internal/rlccclient"
	"qac/internal/seed"
	"qac/internal/server"
	"qac/internal/sourcegraph"
	"qac/internal/store"
	"qac/internal/vm"
)

func serveCmd() *cobra.Command {
	var addr, dbPath, vmBaseURL string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDB, err := resolveDBPath(dbPath)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			s, err := store.Open(ctx, resolvedDB)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()

			if err := seed.LoadEmbeddedTemplates(ctx, s, qac.TemplatesFS); err != nil {
				return fmt.Errorf("seed templates: %w", err)
			}

			// RLCC + FLCC backend selection. Token + override are read at
			// startup; missing token falls through to MapClient without
			// erroring. Set AWXCTL_SOURCEGRAPH_TOKEN (e.g. in a local .env)
			// before invoking qac for live reads.
			token := os.Getenv("AWXCTL_SOURCEGRAPH_TOKEN")
			backendOverride := os.Getenv("QAC_RLCC_BACKEND")
			var rlccC rlccclient.Client
			var flccC flccclient.Client
			var liveBackend bool
			var purgeSourcegraph = func() {}
			var sgRefresh func() // nil unless we have a token to refresh with

			switch {
			case backendOverride == "map":
				slog.Info("using map backend per QAC_RLCC_BACKEND=map")
				rlccC = rlccclient.NewMapClient()
				flccC = flccclient.NewMapClient()
			default:
				cacheDir := filepath.Join(filepath.Dir(resolvedDB), "sourcegraph")
				sg, err := sourcegraph.NewClientWithCache("https://coreweave.sourcegraphcloud.com", cacheDir)
				if err != nil {
					return fmt.Errorf("sourcegraph client: %w", err)
				}
				const (
					rlccRepo = "github.com/coreweave/rack-lifecycle-controller"
					rlccPath = "chart/rack-lifecycle-controller/values.yaml"
					rlccRef  = "main"
					flccRepo = "github.com/coreweave/fleet-lifecycle-controller"
					flccPath = "chart/values.yaml"
					flccRef  = "develop"
				)
				haveCopy := sg.Has(rlccRepo, rlccPath, rlccRef) || sg.Has(flccRepo, flccPath, flccRef)
				if token == "" && !haveCopy {
					slog.Warn("using map backend; no Sourcegraph token and no local copy (set AWXCTL_SOURCEGRAPH_TOKEN in .env to fetch and cache the real workflows)")
					rlccC = rlccclient.NewMapClient()
					flccC = flccclient.NewMapClient()
					break
				}
				rlccC = rlccclient.NewSourcegraphClient(sg)
				flccC = flccclient.NewSourcegraphClient(sg)
				liveBackend = true
				purgeSourcegraph = sg.PurgeCache
				if token != "" {
					slog.Info("using sourcegraph backend (disk-cached)", "cache_dir", cacheDir, "token_len", len(token))
					sgRefresh = func() {
						refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer refreshCancel()
						if _, err := sg.Refresh(refreshCtx, rlccRepo, rlccPath, rlccRef); err != nil {
							slog.Warn("rlcc refresh failed", "err", err)
						}
						if _, err := sg.Refresh(refreshCtx, flccRepo, flccPath, flccRef); err != nil {
							slog.Warn("flcc refresh failed", "err", err)
						}
					}
				} else {
					slog.Info("serving local Sourcegraph copy (no token this session; will not refresh)", "cache_dir", cacheDir)
				}
			}

			// Shared VM client: fans out across all super-regions with
			// per-request vmui fallback. Creds present → authed VMauth;
			// creds absent → unauthed vmui (real data) + a one-time
			// warning. A non-empty --vm-url pins a single authed
			// "override" super-region (testing / escape hatch); empty
			// uses the built-in four-region maps. One client is shared by
			// the inventory resolver and the lifecycle client.
			vmCfg := vm.Config{
				Username: os.Getenv("AWXCTL_VMAUTH_USERNAME"),
				Password: os.Getenv("AWXCTL_VMAUTH_PASSWORD"),
			}
			if vmBaseURL != "" {
				vmCfg.AuthedURLs = map[string]string{"override": vmBaseURL}
				// The override endpoint is authed-only (no vmui fallback);
				// without creds every query to it fails. Warn rather than
				// fail silently — this combination is an operator mistake.
				if vmCfg.Username == "" || vmCfg.Password == "" {
					slog.Warn("--vm-url set but VMauth creds absent; all queries to the override endpoint will fail")
				}
			}
			vmClient := vm.New(vmCfg)
			// authed reflects whether the shared client runs against VMauth
			// (creds present) or falls back to unauthed vmui — the vmui
			// warning itself goes to stderr from the vm package, so surface
			// the mode in the structured log too.
			vmAuthed := vmCfg.Username != "" && vmCfg.Password != ""

			// Inventory backend selection. QAC_INV_BACKEND=map forces the
			// demo MapResolver; otherwise the VMResolver resolves any real
			// rack from VictoriaMetrics via the shared fan-out client.
			invBackend := os.Getenv("QAC_INV_BACKEND")
			var resolver inventoryclient.Resolver
			if invBackend == "map" {
				slog.Info("using demo inventory map per QAC_INV_BACKEND=map")
				resolver = inventoryclient.NewMapResolverWithBMNs(inventoryclient.SeedDemoFixtures(), inventoryclient.SeedDemoBMNs())
			} else {
				resolver = inventoryclient.NewVMResolver(vmClient)
				slog.Info("using vm backend for inventory resolution", "authed", vmAuthed)
			}

			// Lifecycle backend selection. QAC_VM_BACKEND=map forces the
			// demo MapClient; otherwise the PromClient queries live
			// lifecycle state via the same shared fan-out client.
			vmBackend := os.Getenv("QAC_VM_BACKEND")
			var lifeC lifecycleclient.Client
			if vmBackend == "map" {
				slog.Info("using map backend per QAC_VM_BACKEND=map")
				lifeC = lifecycleclient.NewMapClient(nil)
			} else {
				lifeC = lifecycleclient.NewPromClient(vmClient)
				slog.Info("using prom backend for lifecycle queries", "authed", vmAuthed)
			}

			// alert_probe (categories) + firmware_inventory_probe gather from
			// VM via the same shared fan-out client, gated by the same
			// QAC_VM_BACKEND=map switch as lifecycle.
			var alertCatC alertcategoryclient.Client
			var firmwareC firmwareclient.Client
			var hpcVerifC hpcverifclient.Client
			var gpuPerfC gpuperfclient.Client
			if vmBackend == "map" {
				alertCatC = alertcategoryclient.NewMapClient(alertcategoryclient.SeedDemoCategories())
				firmwareC = firmwareclient.NewMapClient(firmwareclient.SeedDemoFirmware()).
					WithBundles(firmwareclient.SeedDemoFirmwareBundles())
				hpcVerifC = hpcverifclient.NewMapClient(hpcverifclient.SeedDemoHPCFailures()).
					WithRoster(hpcverifclient.SeedDemoHPCRoster(time.Now().Unix()))
				gpuPerfC = gpuperfclient.NewMapClient(gpuperfclient.SeedDemoGPUPerf())
			} else {
				alertCatC = alertcategoryclient.NewPromClient(vmClient)
				firmwareC = firmwareclient.NewPromClient(vmClient)
				hpcVerifC = hpcverifclient.NewPromClient(vmClient)
				gpuPerfC = gpuperfclient.NewPromClient(vmClient)
			}

			// awx_job_probe shells out to the read-only `awxctl job info`
			// CLI — not VM. QAC_AWX_BACKEND=map forces the demo fixtures;
			// otherwise the CLI backend is used when awxctl is on $PATH,
			// auto-falling-back to map when it isn't (CI / dev machines).
			awxBackend := os.Getenv("QAC_AWX_BACKEND")
			var awxJobC awxclient.Client
			if awxBackend == "map" || (awxBackend == "" && !awxclient.Available()) {
				if awxBackend == "" {
					slog.Warn("awxctl not found on PATH; using awx map backend for awx_job_probe (set QAC_AWX_BACKEND=cli to force the CLI)")
				} else {
					slog.Info("using awx map backend per QAC_AWX_BACKEND=map")
				}
				awxJobC = awxclient.NewMapClient(awxclient.SeedDemoAWXJobs())
			} else {
				awxJobC = awxclient.NewCLIClient("")
				slog.Info("using awxctl CLI backend for awx_job_probe")
			}

			// Loki client for the l11-fielddiag branch (rack-wide job link via
			// RLCC logs). Uses the Grafana datasource proxy + a Grafana
			// service-account token (GRAFANA_SERVICE_ACCOUNT). Falls back to the
			// demo map backend when in map mode or the token is absent.
			grafanaToken := os.Getenv("GRAFANA_SERVICE_ACCOUNT")
			var lokiC lokiclient.Client
			if awxBackend == "map" || grafanaToken == "" {
				if awxBackend != "map" {
					slog.Warn("GRAFANA_SERVICE_ACCOUNT unset; using loki map backend for awx_job_probe l11 branch")
				}
				lokiC = lokiclient.NewMapClient(lokiclient.SeedDemoL11Logs())
			} else {
				lokiC = lokiclient.NewHTTPClient(lokiclient.Config{Token: grafanaToken})
				slog.Info("using grafana/loki backend for awx_job_probe l11 branch")
			}

			reg := probe.NewRegistry()
			reg.Register(alertprobe.New())
			reg.Register(firmwareinventoryprobe.New())
			reg.Register(hpcverifprobe.New())
			reg.Register(gpuperfprobe.New())
			reg.Register(awxjobprobe.New())
			reg.Register(rlccactionprobe.New())
			reg.Register(failcauseprobe.New())
			eng := engine.New(s, reg, probe.Clients{
				AlertClient:         alertclient.NewMapAlertClient(alertclient.SeedDemoAlerts(), alertclient.SeedDemoDeviceslotAlerts()),
				AlertCategoryClient: alertCatC,
				FirmwareClient:      firmwareC,
				HPCVerifClient:      hpcVerifC,
				GPUPerfClient:       gpuPerfC,
				AWXJobClient:        awxJobC,
				LokiClient:          lokiC,
				InventoryResolver:   resolver,
				LifecycleClient:     lifeC,
				EvidenceWriter:      s,
				EvidenceReader:      s,
				EventEmitter:        probe.NewStoreEmitter(s),
			})

			// Agent LLM backend: live Anthropic when ANTHROPIC_API_KEY is
			// set (e.g. via .env), else the offline fixture. Fixture keeps
			// the flow working and hermetic without a key.
			var llmC llmclient.Client
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				llmC = llmclient.NewAnthropicClient()
				slog.Info("using anthropic backend for agents", "model", llmC.Info().Model)
			} else {
				llmC = llmclient.NewFixtureClient()
				slog.Warn("ANTHROPIC_API_KEY unset; using fixture assessment for agents (no live AI verdict)")
			}
			agentReg := agent.NewRegistry()
			agentReg.Register(canaryassessor.New())
			eng.RegisterAgents(agentReg, agent.Clients{LLM: llmC})

			srv, err := server.New(s, addr, resolvedDB, qac.DistFS, resolver, eng, rlccC, flccC, liveBackend, purgeSourcegraph, lifeC)
			if err != nil {
				return fmt.Errorf("build server: %w", err)
			}

			// Daily Sourcegraph local-copy refresh (only when a token is
			// present). Warms the copy on startup, then refreshes every 24h.
			// stopRefresh is closed in the shutdown path so the goroutine and
			// its ticker stop cleanly — no leaked goroutine.
			stopRefresh := make(chan struct{})
			if sgRefresh != nil {
				go func() {
					sgRefresh() // startup warm (Refresh always re-fetches; bounds the copy to ~now)
					t := time.NewTicker(24 * time.Hour)
					defer t.Stop()
					for {
						select {
						case <-stopRefresh:
							return
						case <-t.C:
							sgRefresh()
							slog.Info("sourcegraph local copy refreshed (24h tick)")
						}
					}
				}()
			}

			// Graceful shutdown on SIGINT/SIGTERM.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			errCh := make(chan error, 1)
			go func() {
				slog.Info("qac serve", "addr", addr, "db", resolvedDB)
				errCh <- srv.ListenAndServe()
			}()

			select {
			case sig := <-sigCh:
				slog.Info("shutting down", "signal", sig.String())
				close(stopRefresh)
				shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
				defer c()
				_ = srv.Shutdown(shutdownCtx)
				_ = eng.Shutdown(shutdownCtx)
				return nil
			case err := <-errCh:
				close(stopRefresh)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "listen address")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite path (default: $XDG_DATA_HOME/qac/qac.db)")
	// --vm-url is an optional single-endpoint override: when set, qac
	// pins one authed "override" super-region instead of the built-in
	// four-region maps. Empty (the default) uses the built-in maps and
	// fans out across us-east / us-west / eu-south / us-lab, with an
	// unauthed vmui fallback when VMauth creds are absent or a query
	// fails. VMauth is plain HTTP (an https:// scheme fails TLS).
	cmd.Flags().StringVar(&vmBaseURL, "vm-url", "", "VictoriaMetrics single-endpoint override (default: built-in super-region maps)")
	return cmd
}

func resolveDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(dataHome, "qac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return filepath.Join(dir, "qac.db"), nil
}
