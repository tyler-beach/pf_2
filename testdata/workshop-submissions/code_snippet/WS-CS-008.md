# Helmet CSP middleware [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/helmetjs/helmet/blob/main/middlewares/content-security-policy/index.ts
  repo: helmetjs/helmet
  path: middlewares/content-security-policy/index.ts
  license: MIT
  retrieved: 2026-10-28
  excerpt_sha256_12: e24d839f300c
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: security, javascript, apis.

### File: csp.ts

```typescript
import type { IncomingMessage, ServerResponse } from "node:http";
import { dashify, errify, isString, throwErrorIfExists } from "./util";

type ContentSecurityPolicyDirectiveValueFunction = (
  req: IncomingMessage,
  res: ServerResponse,
) => string;

type ContentSecurityPolicyDirectiveValue =
  string | ContentSecurityPolicyDirectiveValueFunction;

type ContentSecurityPolicyDirectives = Record<
  string,
  | null
  | Iterable<ContentSecurityPolicyDirectiveValue>
  | typeof dangerouslyDisableDefaultSrc
>;

export type ContentSecurityPolicyOptions = { reportOnly?: boolean } & (
  | { useDefaults?: true; directives?: ContentSecurityPolicyDirectives }
  | { useDefaults: false; directives: ContentSecurityPolicyDirectives }
);

type ParsedDirectivesMap = Map<
  string,
  Iterable<ContentSecurityPolicyDirectiveValue>
>;

type ContentSecurityPolicy = {
  (
    options?: Readonly<ContentSecurityPolicyOptions>,
  ): (
    req: IncomingMessage,
    res: ServerResponse,
    next: (err?: Error) => void,
  ) => void;
  getDefaultDirectives: typeof getDefaultDirectives;
  dangerouslyDisableDefaultSrc: typeof dangerouslyDisableDefaultSrc;
};

const dangerouslyDisableDefaultSrc = Symbol("dangerouslyDisableDefaultSrc");

const SHOULD_BE_QUOTED: ReadonlySet<string> = new Set([
  "none",
  "self",
  "strict-dynamic",
  "report-sample",
  "inline-speculation-rules",
  "unsafe-inline",
  "unsafe-eval",
  "unsafe-hashes",
  "wasm-unsafe-eval",
]);

const getDefaultDirectives = (): Record<
```
