# MADR: Add status field [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/adr/madr/blob/develop/docs/decisions/0008-add-status-field.md
  repo: adr/madr
  path: docs/decisions/0008-add-status-field.md
  license: MIT
  retrieved: 2026-10-28
  excerpt_sha256_12: fc43304e65a2
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: architecture, process.

## Technical specification / ADR (excerpt)

---
parent: Decisions
nav_order: 8
---
# Add Status Field

## Context and Problem Statement

Technical Story: <https://github.com/adr/madr/issues/2>

ADRs have a status. Should this be tracked? And if it should, how should we track it?

## Considered Options

* Use YAML front matter
* Use badge
* Use text line
* Use separate heading
* Use table
* Do not add status

## Decision Outcome

Chosen option: "Use YAML front matter", because comes out best (see below).

## Pros and Cons of the Options

### Use YAML front matter

Example:

```markdown
---
parent: Decisions
nav_order: 3
status: on hold
---
# Write own MADR tooling
```

* Good, because YAML front matter is supported by most Markdown parsers

### Use badge

#### Examples

* ![Example "Use Angular" with "status: accepted"](0008-example-badge.png)
* [![Example "status: superseded"](https://img.shields.io/badge/status-superseeded_by_ADR_0001-orange.svg?style=flat-square)](https://github.com/adr/madr/blob/develop/docs/decisions/0001-use-CC0-or-MIT-as-license.md)

---

* Good, because plain markdown
* Good, because looks good
* Bad, because hard to read in markdown source
* Bad, because relies on the online service <https://shields.io> or [local badges have to be generated](https://github.com/badges/shields#using-the-badge-library)

