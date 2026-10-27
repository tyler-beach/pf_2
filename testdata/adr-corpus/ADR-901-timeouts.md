# ADR-901: Explicit service timeouts

## Status

Accepted

## Context

Unbounded outbound calls can exhaust worker capacity and increase queue latency.

## Decision

Every outbound network call must define a finite timeout.
Service clients should apply bounded retries with jitter.
An exception may be approved only when a documented interoperability constraint prevents a timeout.

## Consequences

Implementations must expose timeout values in configuration.

## Example

~~~go
// must in a code example is not an obligation.
const example = "must not be extracted";
~~~
