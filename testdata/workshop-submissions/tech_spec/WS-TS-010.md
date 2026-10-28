# Backstage: Use fetch [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/backstage/backstage/blob/master/docs/architecture-decisions/adr014-use-fetch.md
  repo: backstage/backstage
  path: docs/architecture-decisions/adr014-use-fetch.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 94bf101126a1
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: networking, apis.

## Technical specification / ADR (excerpt)

---
id: adrs-adr014
title: 'ADR014: Proper use of HTTP fetching libraries'
description: Architecture Decision Record (ADR) for the proper use of fetchApiRef, native fetch, and cross-fetch for data fetching.
---

## Context

Until now we have been recommending the use of `node-fetch` in Node.js contexts
through [ADR013](./adr013-use-node-fetch.md). Since then, Backstage has had its
minimum requirements upgraded to Node.js 20 or newer. The Node.js platform has
established a stable, reliable `undici` based native `fetch` in these versions.
Additionally, there are [some issues](https://github.com/backstage/backstage/issues/24590)
with using third party libraries that only appeared in newer versions of
Node.js.

## Decision

All code that is executed in Node.js (including backend and CLIs) should use the
native `fetch` for HTTP data fetching, and `typeof fetch` as the TypeScript type
in code where a `fetch` implementation can be injected or is referred to.
Example:

```ts
import { ResponseError } from '@backstage/errors';

// this is implicitly global.fetch
const response = await fetch('https://example.com/api/v1/users.json');
if (!response.ok) {
  throw await ResponseError.fromResponse(response);
}
const users = await response.json();
```

Frontend plugins and packages should prefer to use the
[`fetchApiRef`](https://backstage.io/api/stable/variables/_backstage_core-plugin-api.index.fetchApiRef.html).

```ts
import { useApi } from '@backstage/core-plugin-api';

// Inside some React component...
const { fetch } = useApi(fetchApiRef);

const response = await fetch('https://example.com/api/v1/users.json');
if (!response.ok) {
  throw await ResponseError.fromResponse(response);
}
const users = await response.json();
```

Isomorphic packages should have a dependency on the `cross-fetch` package for
mocking and type definitions. Preferably, classes and functions in isomorphic
packages should accept an argument of type `typeof fetch` to let callers supply
their preferred implementation of `fetch`. This lets them adorn the calls with
auth or other information, and track metrics etc, in a cross-platform way.

