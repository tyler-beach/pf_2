# Flux RFC-0001 authorization [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/fluxcd/flux2/blob/main/rfcs/0001-authorization/README.md
  repo: fluxcd/flux2
  path: rfcs/0001-authorization/README.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 1c3c6624be44
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: gitops, auth, rbac.

## Technical specification / ADR (excerpt)

# RFC-0001 Memorandum on Flux Authorization

## Summary

This RFC describes in detail, for [Flux version 0.24][] (Nov 2021), how Flux determines which
operations are allowed to proceed, and how this interacts with Kubernetes' access control.

## Motivation

To this point, the Flux project has provided [examples of how to make a multi-tenant
system](https://github.com/fluxcd/flux2-multi-tenancy/tree/v0.1.0), but not explained exactly how
they relate to Flux's authorization model; nor has the authorization model itself been
documented. Further work on support for multi-tenancy, among other things, requires a full account
of Flux's authorization model as a baseline.

### Goals

- Give a comprehensive account of Flux's authorization model

### Non-Goals

- Justify the model as it stands; this RFC simply records the state as at v0.24.

## Flux's authorization model

The Flux controllers undertake operations as specified by custom resources of the kinds defined in
the [Flux API][]. Most of the operations are through the Kubernetes API. Authorization for
operations on external systems is not accounted for here.

Flux controllers defer to [Kubernetes' native RBAC][k8s-rbac] and [namespace isolation][k8s-ns] to
determine which operations are authorized, when processing the custom resources in the Flux API.

In general, **Kubernetes API operations are constrained by the service account under which each
controller pod runs**. In the [default deployment of Flux][flux-rbac] each controller has its own
service account; and, the service accounts for the Kustomize controller and Helm controller have the
[`cluster-admin` cluster role][k8s-cluster-admin] bound to it.

Both the Kustomize controller and the Helm controller create, update and delete arbitrary sets of
configuration that they take as user input. For example, a Kustomization object that references a
GitRepository is processed by taking whatever is in the specified Git repository and applying it to
the cluster. This is informally called "syncing", and these user-supplied configurations will be
called "sync configurations" in the following.

There are five types of access that have a distinct treatment with respect to RBAC and namespace
isolation:

 - reading and writing the Flux API object to be processed
 - accessing dependencies of a Flux API object; for example, a secret that holds a decryption key
 - accessing Flux API objects related to the object being processed; for example, a GitRepository
   referenced by a Kustomization
 - creating, updating and deleting Flux API objects as part of processing; for example, each
   `HelmRelease` object contains a template for a Helm chart spec, which the Helm controller uses to
   create a `HelmChart` object
 - creating, updating, deleting, and health-checking of arbitrary objects as specified by _sync
   configurations_ (as mentioned above).

