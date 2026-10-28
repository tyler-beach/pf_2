# Flux RFC-0007 passwordless Git auth [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/fluxcd/flux2/blob/main/rfcs/0007-git-repo-passwordless-auth/README.md
  repo: fluxcd/flux2
  path: rfcs/0007-git-repo-passwordless-auth/README.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 8f9d915f26e1
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: gitops, auth.

## Technical specification / ADR (excerpt)

# RFC-0007 Passwordless authentication for Git repositories

**Status:** implementable

**Creation date:** 2023-31-07
**Last update:** 2025-08-13

## Summary

Flux should provide a mechanism to authenticate against Git repositories without
the use of passwords. This RFC proposes the use of alternative authentication
methods like OIDC, OAuth2 and IAM to access Git repositories hosted on specific
Git SaaS platforms and cloud providers.

## Motivation

At the moment, Flux supports HTTP basic and bearer authentication. Users are
required to create a Secret containing the username and the password/bearer
token, which is then referred to in the GitRepository using `.spec.secretRef`.

While this works fine, it has a couple of drawbacks:
* Scalability: Each new GitRepository potentially warrants another credentials
pair, which doesn't scale well in big organizations with hundreds of
repositories with different owners, increasing the risk of mismanagement and
leaks.
* Identity: A username is associated with an actual human. But often, the
repository belongs to a team of 2 or more people. This leads to a problem where
teams have to decide whose credentials should Flux use for authentication.

These problems exist not due to flaws in Flux, but because of the inherent
nature of password based authentication.

With support for OIDC, OAuth2 and IAM based authentication, we can eliminate
these problems:
* Scalability: Since OIDC is fully handled by the cloud provider, it eliminates
any user involvement in managing credentials. For OAuth2 and IAM, users do need
to provide certain information like the ID of the resource, private key, etc.
but these are still a better alternative to passwords since the same resource
can be reused by multiple teams with different members.
* Identity: Since all the above authentication methods are associated with a
virtual resource independent of a user, it solves the problem of a single person
being tied to automation that several people are involved in.

### Goals

* Integrate with major cloud providers' OIDC and IAM offerings to provide a
seamless way of Git repository authentication.
* Integrate with major Git SaaS providers to support their app based OAuth2
mechanism.

### Non-Goals

* Replace the existing basic and bearer authentication API.

## Proposal

