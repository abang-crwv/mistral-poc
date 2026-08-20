# qac

Self-contained Go binary serving a React SPA for firmware-release canary verification.

Standalone project at `/Users/wpena/coreweave/qac` (formerly developed inside
`fleet-ops-sandbox/team/wpena/qac/`).

See `docs/superpowers/specs/2026-05-27-qac-rebuild-design.md` for the design.

## Quickstart

```bash
make build         # builds frontend + backend, output to bin/qac
./bin/qac seed-demo
./bin/qac serve --addr 127.0.0.1:8080
# open http://127.0.0.1:8080
```

## Development

```bash
make dev           # vite on :5173 proxying /api → :8080 (Go)
make test          # go test ./... && yarn --cwd web test
make lint          # go vet ./... && yarn --cwd web lint
```

## Local-dev credentials (`.env`)

qac loads a `.env` file from the working directory at startup (it is gitignored —
never commit it; `pr-security` scans for secrets). Real environment variables
always take precedence over `.env`. Only the creds qac actually consumes are
read; leave a value blank to keep the offline/fixture behavior for that backend.

```sh
# .env  (in the qac project dir)
AWXCTL_SOURCEGRAPH_TOKEN=     # RLCC + FLCC workflow catalogs (New-run picker); blank → vendored fixture / cached copy
AWXCTL_VMAUTH_USERNAME=       # VictoriaMetrics basic-auth — live lifecycle/FLCC state
AWXCTL_VMAUTH_PASSWORD=
ANTHROPIC_API_KEY=            # AI-reasoned verdict at the ai_assess step; blank → offline fixture assessment
```

With `AWXCTL_SOURCEGRAPH_TOKEN` set, qac fetches the real RLCC/FLCC workflows,
caches them under `$XDG_DATA_HOME/qac/sourcegraph/`, and refreshes daily — so
later tokenless runs still serve the real list.

With `ANTHROPIC_API_KEY` set, the `ai_assess` step reasons over gathered
evidence with Claude Opus 4.8 and emits an advisory verdict; without it, a
deterministic fixture assessment is used.
