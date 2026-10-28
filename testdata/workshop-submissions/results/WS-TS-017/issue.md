# Flux RFC-0010 multi-tenant workload identity [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/fluxcd/flux2/blob/main/rfcs/0010-multi-tenant-workload-identity/README.md
  repo: fluxcd/flux2
  path: rfcs/0010-multi-tenant-workload-identity/README.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 92ad0de480f7
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: gitops, auth, aws.

## Technical specification / ADR (excerpt)

# RFC-0010 Multi-Tenant Workload Identity

**Status:** implemented

**Creation date:** 2025-02-22

**Last update:** 2026-03-13

## Summary

In this RFC we aim to add support for multi-tenant workload identity in Flux,
i.e. the ability to specify at the object-level which set of cloud provider
permissions must be used for interacting with the respective cloud provider
on behalf of the reconciliation of the object. In this process, credentials
must be obtained automatically, i.e. this feature must not involve the use
of secrets. This would be useful in a number of Flux APIs that need to
interact with cloud providers, spanning all the Flux controllers.

### Multi-Tenancy Model

In the context of this RFC, multi-tenancy refers to the ability of a single
Flux instance running inside a Kubernetes cluster to manage Flux objects
belonging to all the tenants in the cluster while still ensuring that each
tenant has access only to their own resources according to the Least Privilege
Principle. In this scenario a tenant is often a team inside an organization,
so the reader can consider the
[multi-team tenancy model](https://kubernetes.io/docs/concepts/security/multi-tenancy/#multiple-teams).
Each team has their own namespaces, which are not shared with other teams.

## Motivation

Flux has strong multi-tenancy features. For example, the `Kustomization` and
`HelmRelease` APIs support the field `spec.serviceAccountName` for specifying
the Kubernetes `ServiceAccount` to impersonate when interacting with the
Kubernetes API on behalf of a tenant, e.g. when applying resources. This
allows tenants to be constrained under the Kubernetes RBAC permissions
granted to this `ServiceAccount`, and therefore have access only to the
specific subset of resources they should be allowed to use.

Besides the Kubernetes API, Flux also interacts with cloud providers, e.g.
container registries, object storage, pub/sub services, etc. In these cases,
Flux currently supports basically two modes of authentication:

- *Secret-based multi-tenant authentication*: Objects have the field
  `spec.secretRef` for specifying the Kubernetes `Secret` containing the
  credentials to use when interacting with the cloud provider. This is
  similar to the `spec.serviceAccountName` field, but for cloud providers.
  The problem with this approach is that secrets are a security risk and
  operational burden, as they must be managed and rotated.
- *Workload-identity-based single-tenant authentication*: Flux offers
  single-tenant workload identity support by configuring the `ServiceAccount`
  of the Flux controllers to impersonate a cloud identity. This eliminates
  the need for secrets, as the credentials are obtained automatically by
  the cloud provider Go libraries used by the Flux controllers when they
  are running inside the respective cloud environment. The problem with

