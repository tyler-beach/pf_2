# Backstage: Plugin package structure [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/backstage/backstage/blob/master/docs/architecture-decisions/adr011-plugin-package-structure.md
  repo: backstage/backstage
  path: docs/architecture-decisions/adr011-plugin-package-structure.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 435589c8feae
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: platform, modularity.

## Technical specification / ADR (excerpt)

---
id: adrs-adr011
title: 'ADR011: Plugin Package Structure'
description: Architecture Decision Record (ADR) for Plugin Package Structure
---

## Context

A core feature of Backstage is the extensibility via plugins. Even most of the core features
are implemented as plugins. A plugin consists of one or multiple packages in the
`plugins/` directory. Up till now, we have a simple conventions for naming
plugin packages: Plugins are named `x`, with the option of having a related
backend plugin called `x-backend` (where `x` is the plugin name, like `catalog`
or `techdocs`). There is a need for sharing code between the frontend and
backend of a plugin, between backend plugins, or components and hooks between
different frontend plugins
([some examples](https://github.com/backstage/backstage/issues/3655#issuecomment-758166746)).
This results in emerging plugin packages with shared code, like
`packages/catalog-client` or `packages/techdocs-common`.

> There is a common phrase in software development:
> [Naming things is hard](https://martinfowler.com/bliki/TwoHardThings.html)

To keep the contributed plugins consistent, this Architecture Decision Record
provides rules for naming plugin packages.

## Decision

We will place all plugin related code in the `plugins/` directory. The
`packages/` directory is reserved for core package of Backstage.

We follow this structure for plugin packages (where `x` is the plugin name, for
example `catalog` or `techdocs`):

- `x`: Contains the main frontend code of the plugin.
- `x-module-<name>`: Contains optional modules related to the frontend plugin
  package.
- `x-backend`: Contains the main backend code of the plugin.
- `x-backend-module-<name>`: Contains optional modules related to the backend
  plugin package.
- `x-react`: Contains shared widgets, hooks and similar that both the plugin
  itself (`x`) and third-party frontend plugins can depend on.
- `x-node`: Contains utilities for backends that both the plugin backend itself
  (`x-backend`) and third-party backend plugins can depend on.
- `x-common`: An isomorphic package with platform agnostic models, clients, and
  utilities that all packages above or any third-party plugin package can depend
  on.

We prefix the package names with `@backstage/plugin-`.

This structure is based on a
[suggestion in issue #3655](https://github.com/backstage/backstage/issues/3655#issuecomment-758166746).

## Consequences

