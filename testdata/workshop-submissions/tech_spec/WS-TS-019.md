# containerd garbage collection [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/containerd/containerd/blob/main/docs/garbage-collection.md
  repo: containerd/containerd
  path: docs/garbage-collection.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: eba558bddf1d
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: reliability, runtime.

## Technical specification / ADR (excerpt)

# Garbage Collection

`containerd` has a garbage collector capable of removing resources which are no
longer being used. The client is responsible for ensuring that all resources
which are created are either used or held by a lease at all times, else they
will be considered eligible for removal. The Go client library
(`github.com/containerd/containerd/v2/client`) has built-in behavior to ensure resources
are properly tracked and leased. However, the lifecycles of leases are the
responsibility of the caller of the library. The `containerd` daemon has strict
resource management and will garbage collect any unused resource.

## What is a lease?

Leases are a resource in `containerd` which are created by clients and are used
to reference other resources such as snapshots and content. Leases may be
configured with an expiration or deleted by clients once it has completed an
operation. Leases are needed to inform the `containerd` daemon that a resource
may be used in the future after the client completes an operation, even though
it currently is not seen as utilized.

## How to use leases

### using Go client

The best way to use leases is to add it to a Go context immediately after the
context is created. Normally the lifespan of a lease will be the same as the
lifecycle of a Go context.

```.go
	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)
```

This will create a lease which will defer its own deletion and have a default
expiry of 24 hours (in case the process dies before defer). For most use cases,
this is enough and no more thought needs to be put into leases.

_But, of course, more complicated use cases are supported..._

If the program or lease are intended to be longer lived, instead of the very
easy `client.WithLease`, the lease manager can be used directly. This also
allows for setting custom labels on the lease or manipulating its resources.
Use `client.LeasesService()` to get a [lease Manager](https://godoc.org/github.com/containerd/containerd/v2/leases#Manager)
which can be used to create, list, and delete leases as well as manage the
referenced resources for that lease.

```.go
	manager := client.LeasesService()

	// this lease will never expire
	// Use `leases.WithExpiration` to make it expire
	// Use `leases.WithLabels` to apply any labels

