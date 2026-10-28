# Helm HIP-0001 process [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/helm/community/blob/main/hips/hip-0001.md
  repo: helm/community
  path: hips/hip-0001.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 160cc00bab4f
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: process, iac.

## Technical specification / ADR (excerpt)

---
hip: "0001"
title: "Writing a HIP"
authors: [ "Matthew Fisher <matt.fisher@microsoft.com>" ]
created: "2020-08-13"
type: "process"
status: "accepted"
---

## Abstract

A HIP is a Helm Improvement Proposal. It is a design document providing
information to the Helm community, or describing a new feature for a Helm
project or its processes or environment. The proposal should provide a concise
technical specification of the feature and a rationale for the feature.

We intend HIPs to be the primary mechanisms for proposing major new features,
for collecting community input on an issue, and for documenting the design
decisions that have gone into the project. The proposal author is responsible
for building consensus within the community and documenting dissenting opinions.

The system is heavily influenced by Python's Enhancement Proposal (PEP) system,
hence the name. Given that Helm is a much (MUCH) smaller project than Python,
there are a few changes in the HIP workflow to make it a looser format for
proposals, most notably the lack of proposal "editors", a BDFL, and a much
smaller review & resolution cycle.

Because the proposals are maintained as markdown files in a versioned
repository, their revision history is the historical record of the proposal.

## Audience

The typical primary audience for proposals are the project maintainers for each
Helm project, as well as the Helm project's org maintainers.

However, other parts of the Helm community may also choose to use the process
(particularly for informational proposals) to document expected conventions and
to manage complex design coordination problems that require collaboration across
multiple projects.

## HIP types

There are three kinds of proposals:

1. A **feature** proposal describes a new feature or implementation for the Helm
   project. It may also describe an interoperability standard that will be
   supported by the Helm project.
1. An **informational** proposal describes a Helm design issue, or provides
   general guidelines or information to the Helm community, but does not propose
   a new feature. Informational proposals do not necessarily represent a Helm
   community consensus or recommendation, so users and implementers are free to
   ignore informational proposals or follow their advice.
1. A **process** proposal describes a process surrounding the Helm project, or
   proposes a change to (or an event in) a process. Process proposals are like
   feature proposals, but apply to areas other than the Helm project itself.

