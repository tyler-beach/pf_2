# Backstage: Catalog core entities [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/backstage/backstage/blob/master/docs/architecture-decisions/adr005-catalog-core-entities.md
  repo: backstage/backstage
  path: docs/architecture-decisions/adr005-catalog-core-entities.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 98740bc5c144
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: platform, data.

## Technical specification / ADR (excerpt)

---
id: adrs-adr005
title: 'ADR005: Catalog Core Entities'
description: Architecture Decision Record (ADR) log on Catalog Core Entities
---

## Context

We want to standardize on a few core entities that we are tracking in the
Backstage catalog. This allows us to build specific plugins around them.

## Decision

Backstage should eventually support the following core entities:

- **Components** are individual pieces of software
- **APIs** are the boundaries between different components
- **Resources** are physical or virtual infrastructure needed to operate a
  component

![Catalog Core Entities](../assets/architecture-decisions/catalog-core-entities.png)

For now, we'll start by only implementing support for the Component entity in
the Backstage catalog. This can later be extended to APIs, Resources and other
potentially useful entities.

### Component

A component is a piece of software, for example a mobile application feature,
web site, backend service or data pipeline (list not exhaustive). A component
can be tracked in source control, or use some existing open source or commercial
software. It can implement APIs for other components to consume. In turn it
might depend on APIs implemented by other components, or resources that are
attached to it at runtime.

Component entities are typically defined in YAML descriptor files next to the
code of the component, and could look like this (actual schema will evolve):

```yaml
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: my-component-name
spec:
  type: service
```

### API

APIs form an abstraction that allows large software ecosystems to scale. Thus,
APIs are a first class citizen in the Backstage model and the primary way to
discover existing functionality in the ecosystem.

APIs are implemented by components and make their boundaries explicit. They
might be defined using an RPC IDL (e.g. in Protobuf, GraphQL or similar), a data

