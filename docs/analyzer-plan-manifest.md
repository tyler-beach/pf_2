# Deterministic analyzer plan manifests

## Purpose

A plan is a reproducible description of the work an analyzer would run against a repository snapshot. It is kept separate from execution output so it can be reviewed, compared, verified, and handed to a local executor or future cloud scheduler without importing cloud SDKs into the assessment engine.

## Invariants

- Identical source/configuration bytes and the same task specification produce byte-identical manifest JSON.
- The root input-tree digest, normalized source inventory, configuration SHA-256, analyzer name/version, scope, and selected source paths contribute to the task cache key.
- The idempotency key additionally binds execution-policy settings: timeout, maximum attempts, output cap, retry backoff, and retryable failure kinds.
- Cache/idempotency keys are lowercase SHA-256 hex digests. Task IDs derive from the idempotency key, not a timestamp or random value.
- Scope paths are repository-relative, sorted, and normalized. Symlinked configurations and root-escaping paths are rejected.
- Empty input scopes are explicit and should not be interpreted as a clean scan.
- Manifest SHA-256 covers the canonical JSON representation with the digest field omitted. The verifier reconstructs the plan from current inputs and rejects stale or manually edited values.
- Retries are bounded to five retries, output is capped at 512 MiB, per-attempt timeouts at 24 hours, and global planned concurrency at sixteen tasks.

## Execution and cache

`execute-plan` is the local execution path for a verified manifest. It rechecks the manifest against the current repository snapshot before dispatch, avoids launching tasks whose declared scope is empty, bounds worker concurrency, applies per-attempt timeouts, and retries only failure kinds explicitly allow-listed by the plan.

For each task, the runner preserves the original SARIF, validates the analyzer identity/version, source-tree digest, configuration digest, and scanned-path inventory, then publishes an execution manifest and one raw SARIF artifact per completed task. The executor passes only the task's declared source paths to the analyzer, and scoped verification requires the reported inventory to match that exact sorted list. Symlinked paths, generated artifacts, excluded analyzer fixtures, configuration files, and paths outside the current eligible source inventory are rejected. Partial evidence, malformed output, unsupported analyzer adapters, cache corruption, and stale source are never silently converted to a clean result. The current production runner supports the pinned OpenGrep adapter; other analyzer names fail closed until a runner is registered.

Use a cache directory outside the repository to avoid changing the input-tree digest between plan creation and execution:

```sh
go run ./cmd/adrctl plan-repo \
  --root . \
  --analyzer opengrep \
  --version 1.33.0 \
  --config rules/pathfinder-security.yml \
  --scope . \
  --json-out analyzer-evidence/plan.json

go run ./cmd/adrctl verify-plan --root . --manifest analyzer-evidence/plan.json

go run ./cmd/adrctl execute-plan \
  --root . \
  --manifest analyzer-evidence/plan.json \
  --cache-dir "$HOME/.cache/pathfinder/analyzers" \
  --json-out analyzer-evidence/execution.json \
  --sarif-dir analyzer-evidence/raw
```

Only complete, source-bound evidence is cacheable. A corrupt cache entry fails closed and does not trigger an unverified fallback. A cache write/read failure, timeout, malformed output, or incomplete scope is represented as a run/task failure, not a pass. Cache keys and idempotency keys include the source/configuration/analyzer and execution policy inputs relevant to reproducibility.

The execution layer is local and cloud-SDK-free. AWS scheduling, durable shared evidence storage, and multi-repository orchestration remain deployment work; this package does not claim they are implemented.

## Package ownership

`internal/plan` is the sole planner/executor for repository analyzer work. `scan-opengrep` builds and executes through the same manifest + cache contracts as `plan-repo` / `execute-plan`. `internal/analyzer` owns OpenGrep invocation, SARIF parse/normalize, and digest verification, and participates as a registered `TaskRunner`. Do not reintroduce a parallel cache/plan implementation inside `internal/analyzer`.
