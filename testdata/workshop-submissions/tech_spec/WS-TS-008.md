# Backstage: Entity references [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/backstage/backstage/blob/master/docs/architecture-decisions/adr009-entity-references.md
  repo: backstage/backstage
  path: docs/architecture-decisions/adr009-entity-references.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 1894ad15682e
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: platform, apis.

## Technical specification / ADR (excerpt)

---
id: adrs-adr009
title: 'ADR009: Entity References'
description: Architecture Decision Record (ADR) log on Entity References
---

## Background

While the spec for the catalog file format is well described in
[ADR002](./adr002-default-catalog-file-format.md), guidance was not provided as
to how one is expected to express references to other entities in the catalog.
There was also some confusion on how to reference entities in URLs in the
Backstage frontend.

Following discussion in
[Issue 1947](https://github.com/backstage/backstage/issues/1947), a decision was
made.

## Entity References in YAML files

The textual format, as written by humans, to reference entities by name is on
the following form, where square brackets denote optionality:

```text
[<kind>:][<namespace>/]<name>
```

That is, it is composed of between one and three parts in this specific order,
without any additional encoding, with those exact separator characters.
Optionality of `kind` and `namespace` are contextual, and they may or may not
have default contextual fallback values.

When that format is insufficient or when machine made interchange formats wish
to express such relations in a more expressive form, a nested structure on the
following form can be used:

```yaml
kind: <kind>
namespace: <namespace>
name: <name>
```

Of these, only `name` is always required. Optionality of `kind` and `namespace`
are contextual, and they may or may not have default contextual fallback values.
All other possible key values in this structure are reserved for future use.

A system or user wanting to express a full entity name that is always valid
shall supply the entire triplet, whether using the string form or the compound
form.

A full description of the format can be found
[in the documentation](https://backstage.io/docs/features/software-catalog/references).

## Entity References in URLs

