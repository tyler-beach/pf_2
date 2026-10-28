# Backstage: Default catalog file format [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/backstage/backstage/blob/master/docs/architecture-decisions/adr002-default-catalog-file-format.md
  repo: backstage/backstage
  path: docs/architecture-decisions/adr002-default-catalog-file-format.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: abaaab065cb3
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: platform, data.

## Technical specification / ADR (excerpt)

---
id: adrs-adr002
title: 'ADR002: Default Software Catalog File Format'
description: Architecture Decision Record (ADR) log on Default Software Catalog File Format
---

## Background

Backstage comes with software catalog functionality that you can use to track
all your software components and more. It can be powered by data from various
sources, and one of them that is included with the package is a custom database
backed catalog. It has the ability to keep itself updated automatically based on
the contents of little descriptor files in your version control system of
choice. Developers create these files and maintain them side by side with their
code, and the catalog system reacts accordingly.

This ADR describes the default format of these descriptor files.

### Inspiration

Internally at Spotify, a homegrown software catalog system is used heavily and
forms a core part of Backstage and other important pieces of the infrastructure.
The user experience, learnings, and certain pieces of metadata from that catalog
are being carried over to the open source effort.

The file format described herein also draws heavy inspiration from the
[kubernetes object format](https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/).

### Core Concepts

There are a number of descriptor files, all of whose locations (e.g., within a
version control system) are registered with the software catalog. The method of
registration is not covered in this document; it could happen either manually
inside Backstage, or by push events from a CI/CD pipelines, or by webhook
triggers from the version control system, etc.

Each file describes one or more entities in accordance with the
[Backstage System Model](https://github.com/backstage/backstage/issues/390). All
of these entities have a common structure and nomenclature, and they are stored
in the software catalog from which they then can be queried.

Entities have distinct names, and they may reference each other by those names.

## Format

Descriptor files use the [YAML](https://yaml.org/spec/1.2/spec.html) format.
They may be written by hand or created using automated tools. Each file may
consist of several YAML documents (separated by `---`), where each document
describes a single entity.

This is an example entity definition with some mocked data.

```yaml
---
apiVersion: backstage.io/v1alpha1

