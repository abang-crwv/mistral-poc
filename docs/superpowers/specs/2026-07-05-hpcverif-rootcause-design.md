# HPC-verification root-cause — Gamble/pod-log evidence + hpc-debug agent

**Date:** 2026-07-05
**Status:** proposed (design)
**Branch:** wp/add-qac

## Problem

`hpc_verification_failure_probe` (even after the coverage+freshness upgrade —
see `2026-07-05-hpcverif-coverage-freshness-design.md`) reports *that* a node
failed HPC verification and its coarse testcase, sourced from
`annotation_hpc_verification_coreweave_cloud_message`. But **the annotation is a
summary — it does not carry the complete errors.** The authoritative failure
detail lives in the **Gamble/NHC pod logs** (Loki) and the structured
`hpcv_result_total` results. Without them the probe can say "gpu_blaze failed on
this node" but not *why* (which GPU, which precision, throttle vs correctness,
XID, T.Limit body, MODS code), and it cannot produce a root cause.

The operator wants HPC-verification status to reach **root-cause depth** — the
same output the AMPS/HPCPERFTEST **`hpc-debug`** skill produces.

## Confirmed via Glean (2026-07)

- **The annotation is incomplete; Gamble/NHC pod logs are the authoritative
  source.** From the hpc-debug playbook: *"pull per-GPU blaze GFLOPS from Loki
  logs (the authoritative source): `{namespace=~"hpc-verification|cw-hpc-verification",
  pod="<NHC_POD>"} |~ "gpu_id|mean_gflops"`"*, and *"read the Min T.Limit log
  body, do not rely on error_code alone"* (the gamble errMsg often has
  `error_code=""` even when the cause is known).
- **"Gamble"** is the NHC/HPC-V test framework. Its structured results feed
  `hpcv_result_total` on the GloQL datasource `benz42hlglhj4a`
  (FMT-1272, *"Verification alerts: single source of truth in gamble"*).
  Caveat: some clusters are absent from `hpcv_result_total`, so the annotation
  + pod logs remain necessary.
- **The structured summary the operator wants IS the `hpc-debug` contract**
  (`coreweave/hpc-debug`, mirror `coreweave/perftest-skills/hpc-debug`):
  a read-only agent that queries Prometheus + Loki, walks playbook tables, and
  emits root cause + confidence + differential diagnosis + evidence queries +
  recommendations, validated by `validate.py` against a query cache and checked
  by an adversarial reviewer.

## Architecture — gather vs interpret

The root-cause summary is an **AI-reasoning artifact**, not something a
deterministic Go probe produces. It is qac's **`ai_assess` / agent** layer
(*"probes gather, agent interprets"*). The split:

1. **Probes GATHER** (deterministic, buildable in Go now): the complete
   evidence — annotation + outcome metrics (existing) **+ Gamble/NHC pod logs
   (Loki) + `hpcv_result_total` (GloQL)**.
2. **The agent PRODUCES the summary** (root cause / confidence / differential /
   evidence-queries / recommendations) by **reusing / invoking `hpc-debug`** —
   not by re-implementing differential diagnosis in Go. `hpc-debug` is already
   playbook-driven, validated, adversarially reviewed, and self-improving; the
   frops-agent one-pager frames a cross-team orchestrator that stitches such
   skills together.

## Proposed changes

### Phase 1 — gatherer extension (Go, deterministic)

Add two evidence sources to the HPC-verification gathering, keyed per failing
node:

- **Gamble/NHC pod logs** via the existing `lokiclient` (built for the l11
  branch — reuse it): for a failing node, query
  `{namespace=~"hpc-verification|cw-hpc-verification", pod=~"hpc-verification-nhc-v3-<node>-.*"}`
  and capture the failure log bodies (T.Limit, gpu_blaze G-320x, XID, per-GPU
  `gpu_id`/`mean_gflops`, NCCL, nvbandwidth, Megatron). Capture the **body**,
  not just an error code.
- **`hpcv_result_total`** (GloQL `benz42hlglhj4a`): the structured per-test
  pass/fail rows for the node/domain.

The gatherer stays a gatherer: it records this as evidence (raw log lines +
structured results), no verdict. It is what the agent reasons over.

### Phase 2 — `ai_assess` agent (reuse hpc-debug)

`ai_assess` consumes the gathered evidence and produces the structured summary
by reusing the `hpc-debug` contract (its playbooks, output template, validator,
reviewer). qac supplies the evidence + the node/domain identifier; the agent
returns the report. qac does **not** reimplement the playbook logic.

## Structured summary contract (from hpc-debug)

The output the operator wants, verbatim from `hpc-debug/playbook/persona.md`:

- **Action Summary** — root cause, **Confidence: Confirmed / Likely /
  Suspected**, recommended action, affected (node/domain/test), pass/fail (7d),
  fast discriminators.
- **Differential Diagnosis** table — `Hypothesis | Supporting Evidence |
  Counter-Evidence | Verdict (Root cause / Ruled out)`.
- **Evidence Queries** table — every cited data point → a reproducible
  PromQL/LogQL, its datasource UID, time range, and key result.
- **Recommendations** — next steps in priority order.
- Terminating `root_cause_claim` YAML (one claim per load-bearing statement,
  each citing a query id) for validation.

## Failure taxonomy (playbook categories)

| Category | Examples |
|---|---|
| GPU hardware | DCGM errors, XIDs, ECC failures, thermal/clock throttle, gpu_blaze |
| Compute | Megatron time-per-iter, loss mismatch, NaN, FP8 |
| Memory | nvbandwidth (H2D/D2D/NVLink), cuda_bw, GPU ECC RMA |
| Network | IB bandwidth/latency, NCCL loopback, NVLink P2P, backend misconnect |
| Group tests | MPI job failures (Megatron multi-node, mnubergemm), domain issues |
| NVLink fabric | NVSwitch failures, fabric manager stuck, domain degradation |
| System | CPU perf, power brake, GPU util stuck, nvidia-smi errors |
| Not testing | node not picked up by getnodes, label mismatch, scheduling |

New root causes are added as playbook table entries, not code.

## Reference evidence queries

- Per-GPU blaze (authoritative): `{namespace=~"hpc-verification|cw-hpc-verification", pod="<NHC_POD>"} |~ "gpu_id|mean_gflops"`
- Throttle/T.Limit body: `{namespace=~"hpc-verification|cw-hpc-verification", pod=~"hpc-verification-nhc-v3-<node>-.*"} |~ "T.Limit"`
- Structured results: `hpcv_result_total{...}` (GloQL `benz42hlglhj4a`)
- Do **not** use `increase(hpcv_plugin_gpu_blaze_value_mean_gflops_sum)/increase(..._count)` — cumulative counters that reset on NHC pod restart.

## Risks / open questions

- **Reuse vs reimplement:** strong recommendation to invoke `hpc-debug` rather
  than port its playbooks into qac. Decide the integration (MCP server fed by
  qac evidence? qac shells to `/hpc-debug`? qac's ai_assess embeds the
  contract?). This is the `ai_assess` iter (previously a stub).
- **GloQL coverage gaps:** clusters missing from `hpcv_result_total` must fall
  back to annotation + pod logs (don't treat absence as pass).
- **Pod-log retention / volume:** NHC pod logs are large; scope the LogQL to the
  node + the failure window; capture only failure bodies.
- **Read-only envelope:** all sources are read-only (Prometheus/Loki queries) —
  no cluster mutations, consistent with hpc-debug and qac's gatherer stance.

## Sources

- `coreweave/hpc-debug` (README, `playbook/persona.md`, `single-node-perf-test.md`,
  `reviewer.md`) — the skill + output contract + playbooks.
- FMT-1272 — Gamble as single source of truth → `hpcv_result_total`.
- TSM cluster reports (cluster01/b300/usw9b) — pod-log absence + staleness cases.
- Memory: `project_qac_hpcverif_rootcause_hpcdebug`, `project_qac_ai_assess_mcp`.
