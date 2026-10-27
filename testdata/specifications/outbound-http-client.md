# Outbound HTTP client requirements

## Scope

These constraints apply to every outbound HTTP client, regardless of implementation language.

## Requirements

All outbound HTTP clients must set a finite request timeout no greater than 30 seconds. Automatic retries must be limited to three attempts. Retry delays should use exponential backoff with jitter. An exception may be approved only when a documented interoperability constraint requires it.
