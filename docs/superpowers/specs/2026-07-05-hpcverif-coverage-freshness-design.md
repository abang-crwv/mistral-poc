# hpc_verification_failure_probe — coverage + freshness upgrade

**Date:** 2026-07-05
**Status:** approved (design)
**Branch:** wp/add-qac

## Problem

`hpc_verification_failure_probe` queries only nodes whose
`annotation_hpc_verification_coreweave_cloud_message` is non-OK. So a rack reads
as **healthy whenever the failure list is empty** — but "empty" conflates three
states:

1. all nodes tested and **passed**
2. nodes **never tested** (not picked up by getnodes)
3. nodes whose last run is **stale** (an old OK from before a firmware zap)

For a firmware canary, #2/#3 are the dangerous ones: a freshly-zapped rack that
has not re-verified reads **green** — a false pass on the exact signal the canary
exists to catch. hpc-debug treats staleness as first-class (>7d prod / >2d
fleetops = stopped testing). Confirmed live: the fleet-wide minimum
`last_heartbeat_time` is ~Nov 2023 — a node stale ~20 months that no failure
query would surface.

## Metric ground truth (live, ds fdokero3r2i9se)

- `kube_node_hpc_verification_last_heartbeat_time` — unix-seconds gauge, emitted
  per **tested** node regardless of pass/fail. This is the coverage roster +
  freshness source.
- Carries `node`, `label_node_coreweave_cloud_slot` (deviceslot), `cluster`,
  `cluster_org`, `region`, `zone` — but **no `rack_name`, no `testcase`**. Scope
  to rack via the same physical-topology + k8s-state + FLCC-lifecycle join the
  failures query already uses.
- `single_node_outcome`/`group_node_outcome` ∈ {0,1}; `outcome` ∈ [0,11] (a
  code, not boolean); `cohort_size` ∈ [1,18] (18 = full GB200 rack). Not needed
  for this iteration — failures come from the existing message query.

## Design

Additive. Keep the existing failure path (only source of `testcase` + message);
add a roster path for coverage + freshness.

**Client** (`hpcverifclient.Client`): new method
`VerificationRoster(ctx, rack) ([]NodeStatus, error)` returning one
`NodeStatus{Node, Deviceslot, LastHeartbeatUnix}` per tested node, after the same
scoping as `VerificationFailures`. Empty roster = rack not verified (not an
error). New query file `queries/hpc_verification_roster.promql`. Implemented in
`PromClient` (via `QueryVectorSamples`, value = heartbeat) and `MapClient`
(new `WithRoster` table + `SeedDemoHPCRoster`).

**Probe**: per rack, combine failures + roster and classify each roster node:
- node in the failing set → **failed**
- else heartbeat age ≤ threshold → **passed**
- else → **stale**

Per-rack `Status`: `failed` if any failure, else `stale` if any stale, else
`not_verified` if roster empty, else `passed`. Staleness threshold defaults to
48h, overridable via `sc.Config["staleness_hours"]`. Per-node untested (a node
missing from a partially-tested rack) needs the expected population from
inventory and is **deferred**; rack-level `not_verified` (empty roster) is
detectable now and covers the whole-rack false-green.

**Evidence** (backward compatible — existing fields retained): add per-rack
`status`, `tested_count`, `passed_count`, `stale_count`,
`oldest_heartbeat_age_sec`, `stale_nodes[]`; top-level `any_stale`,
`any_not_verified`.

## Out of scope

- gpuperfclient's dead NCCL registry entries (`nccl_allreduce_average/expected`
  reference non-existent recording rules) — separate follow-up.
- Per-node untested detection within a partially-tested rack (needs inventory
  roster).
- Decoding the `outcome` [0,11] code.

## Testing

Client: MapClient roster (seeded + empty + FailingSourceRack), PromClient roster
query scoping + heartbeat mapping. Probe: passed/failed/stale classification,
not_verified on empty roster, staleness threshold override, healthy-not-error and
source-error paths preserved. Follow the existing table/httptest patterns.
