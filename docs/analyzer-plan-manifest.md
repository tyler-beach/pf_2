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

## Current boundary

`plan-repo` builds the manifest and `verify-plan` validates it. They do not execute analyzers or claim evidence was collected. The next execution layer must check the manifest at dispatch time, enforce the manifest's limits, preserve raw outputs, distinguish retryable transport/process failures from malformed or partial analyzer evidence, and cache only validated complete outputs. AWS scheduling remains outside this package.
