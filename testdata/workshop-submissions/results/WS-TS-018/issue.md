# Grafana k8s-inspired backend architecture [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/grafana/grafana/blob/main/contribute/architecture/k8s-inspired-backend-arch.md
  repo: grafana/grafana
  path: contribute/architecture/k8s-inspired-backend-arch.md
  license: AGPL-3.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 56713207b713
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: platform, apis.

## Technical specification / ADR (excerpt)

# Grafana Kubernetes-inspired backend architecture

Grafana is adopting a Kubernetes-inspired, resource-oriented model for all Grafana core resources (dashboards, folders, datasources, alert rules, ..) where users can interact with these resources by defining their desired state via well-structured objects sent to Grafana's new Resource APIs (`/apis/...`). Grafana then acts upon these declarations to align the system's current state with the requested state.

On top of adopting the declarative, eventually-consistent system design, we are also adopting Kubernetes API's strict approach to URL structures, versioning and schemas. This approach addresses limitations of Grafana's traditional Legacy APIs (`/api/...`), including:

- **Inconsistent designs:** Variations in structure, parameters, and response formats across different endpoints.
- **No versioning:** Difficulty managing breaking changes and API evolution without clear versioning.
- **Absence of schemas:** Lack of machine-readable schemas hindered programmatic interaction, validation, and tooling.
- **Limited extensibility model:** No standardized way for Grafana plugins to extend core APIs or use Grafana database for storing custom configs

The end goal is for the Resource APIs to become the only interface for managing core Grafana resources. We will eventually deprecate and remove all Legacy API endpoints.

## 1. Key Differences from Legacy API Endpoints

- **URL structure & versioning:**
  - **Resource APIs:** Follow Kubernetes conventions (`/apis/<group>/<version>/namespaces/<namespace>/<resource>/<name>`). Includes explicit API versions (e.g., `v0alpha1`, `v1`, `v2beta1`) in the path, allowing for controlled evolution and multiple versions of a resource API to co-exist.
  - **Legacy APIs:** Variable path structures (e.g., `/api/dashboards/uid/:uid`, `/api/ruler/grafana/api/v1/rules/:uid/:uid`). Less explicit versioning.

- **Resource schemas:**
  - **Resource APIs:** Each resource has a well-defined schema (`spec`) and is wrapped with an envelope with common metadata, all APIs come with an always-in-sync OpenAPI spec.
  - **Legacy APIs:** Structures vary

- **Namespacing / org context:**
  - **Resource APIs:** Uses explicit namespaces in the path (`/namespaces/<namespace>/...`) mapping to Grafana Organization IDs in OSS/Enterprise and Grafana Cloud Stack IDs in Grafana Cloud for scoping.
  - **Legacy APIs:** Org context determined implicitly (session, API key), not usually part of the URL structure.

- **Consistency of convenience features:**
  - **Resource APIs:** Convenience features such as resource history, restore and observability-as-code tooling come out of the box; a single implementation of convenience features works across all resources because of the standardization
  - **Legacy APIs:** Different APIs support different convenience features; implementations are feature-specific

- **Error Handling:**
  - **Resource APIs:** Uses Kubernetes error response conventions (HTTP status codes 400, 401/403, 404, 409; structured error body).
  - **Legacy APIs:** Variable error formats; Grafana-specific.

## 2. Core Concepts

- **Resource APIs:** New Grafana API endpoints that mimic the Kubernetes API structure (e.g., `/apis/dashboard.grafana.app/v1alpha1/dashboards`). These are served by Grafana itself and are distinct from any Kubernetes API running on the same system.

- **API implementation approaches:**
  - **Registry Approach (`pkg/registry/apis/...`):** Original method using Go code (`.../register.go` files). While newer resources are primarily defined using CUE in `apps/`, this path is still used for things such as legacy fallbacks for App-defined resources when data for that resource exists outside of unified storage.
  - **Apps Approach (`apps/...`):** The newer, more modular implementation where APIs are defined using CUE in self-contained modules. Each app defines resources in `kinds/*.cue` files and resources are registered via the App SDK. This is the direction Grafana is moving towards for all resources.
- **Unified Storage (`pkg/storage/unified/...`):** Internal abstraction handling resource persistence. It handles the conversion between the Resource API and the underlying storage backend.

