# ADR-900: Service runtime requirements

## Status

Accepted

## Scope

These requirements apply to all outbound HTTP clients and all deployable services in this repository.

## Requirements

- All outbound HTTP clients must set a finite request timeout no greater than 30 seconds.
- Automatic retries must be limited to three attempts.
- Each service must expose a health endpoint.
- An exception may be approved only when an approved, documented interoperability constraint requires it.

## Context

The service fleet includes Go, Python, and TypeScript applications. The configuration location and health-check route may differ by service.

## Example (illustrative only)

```go
// Example only; this is not evidence that a repository satisfies the requirement.
client := &http.Client{Timeout: 30 * time.Second}
```
