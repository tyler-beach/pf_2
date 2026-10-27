# ADR-960: Specification outcome matrix fixture

## Status

Accepted

## Decision

Services must declare timeout_seconds in configuration. Services must not call eval on untrusted input. Prefer asynchronous messaging where appropriate. An exception may be approved only when a documented interoperability constraint requires it. Each service must expose a health endpoint.

## Notes

This fixture exercises compound extraction, preference ambiguity, exception language, and manual-review candidates. It is a target-workflow fixture, not automatic enforcement of arbitrary prose.
