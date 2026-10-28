# Backstage: Add ADR log [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/backstage/backstage/blob/master/docs/architecture-decisions/adr001-add-adr-log.md
  repo: backstage/backstage
  path: docs/architecture-decisions/adr001-add-adr-log.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: f18029a34745
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: platform, process.

## Technical specification / ADR (excerpt)

---
id: adrs-adr001
title: 'ADR001: Architecture Decision Record (ADR) log'
description: Architecture Decision Record (ADR) logs as a reference point for the team
---

## Decision

A decision was made to store ADRs in a log in the project repository.

## Discussion

There is a need to store big decisions made in a log as a reference point for
the team, help with onboarding new members, and give context to others interested
in the project.

## Risks

People stop adding ADRs to the log and context gets lost.

