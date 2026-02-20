# GroupQuota Plugin

## Motivation

When multiple teams share a Volcano cluster, operators often need a lightweight mechanism to prevent one team from continuously dominating scheduling opportunities once it reaches a configured resource budget.

The `groupquota` plugin addresses this by lowering the scheduling priority of jobs in groups whose current allocated resources have reached (or exceeded) a configured quota.

## Scope

- Provide group-level quota-aware ordering through `AddJobOrderFn`.
- Group identity comes from a PodGroup annotation (configurable key).
- Quota limits are configured in scheduler plugin arguments.

## Non-goals

- This plugin does **not** enforce hard admission control.
- This plugin does **not** replace queue-level fairness (`drf`, `capacity`, `proportion`).
- This plugin does **not** preempt running jobs.

## Configuration

Plugin arguments:

- `annotationKey` (string): PodGroup annotation key used as group identity.
  - Default: `example.com/group`
- `resourceMap` (map): resource quota map, e.g.
  - `cpu: "20"`
  - `memory: "64Gi"`
  - `nvidia.com/gpu: "8"`

## Design

During `OnSessionOpen`:

1. Parse plugin arguments.
2. Build per-group usage from currently allocated jobs.
3. Mark groups as over-quota when `usage >= quota` for any configured resource.
4. Register `JobOrderFn` that de-prioritizes jobs from over-quota groups.

## Boundary behavior

- Jobs without PodGroup annotations are not grouped.
- Missing/invalid `resourceMap` entries are skipped (with logs).
- If no valid quota exists, the plugin is effectively neutral.
- Scalar resource accounting uses milli-quantity conversion to match Volcano internal resource representation.

## Compatibility with existing fairness plugins

This plugin should be treated as a policy signal in job ordering, not a replacement for fairness plugins:

- With `drf` / `proportion` / `capacity`: it can be combined as an additional ranking dimension.
- Recommended to validate final tier order and plugin weights in cluster-specific testing.

## Testing

Minimum test coverage should include:

- Group ordering when one group crosses quota.
- Argument parsing with common map formats.
- Scalar resource quantity conversion correctness.
