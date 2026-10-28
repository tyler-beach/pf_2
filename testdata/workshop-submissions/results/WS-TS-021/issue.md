# OPA project introduction / philosophy [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/open-policy-agent/opa/blob/main/README.md
  repo: open-policy-agent/opa
  path: README.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: b39c59212ece
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: policy, auth.

## Technical specification / ADR (excerpt)

# ![logo](./logo/logo-144x144.png) Open Policy Agent

[![Build Status](https://github.com/open-policy-agent/opa/workflows/Post%20Merge/badge.svg)](https://github.com/open-policy-agent/opa/actions) [![CII Best Practices](https://www.bestpractices.dev/projects/1768/badge)](https://www.bestpractices.dev/en/projects/1768/passing) [![Netlify Status](https://api.netlify.com/api/v1/badges/4a0a092a-8741-4826-a28f-826d4a576cab/deploy-status)](https://app.netlify.com/sites/openpolicyagent/deploys)

Open Policy Agent (OPA) is an open source, general-purpose policy engine that enables unified, context-aware policy enforcement across the entire stack.

OPA is proud to be a graduated project in the [Cloud Native Computing Foundation](https://www.cncf.io/) (CNCF) landscape. For details read the CNCF [announcement](https://www.cncf.io/announcements/2021/02/04/cloud-native-computing-foundation-announces-open-policy-agent-graduation/).

## Get started with OPA

- Write your first Rego policy with the [Rego Playground](https://play.openpolicyagent.org) or use it to share your work with others for feedback and support. Have a look at the [Access Control examples](https://play.openpolicyagent.org/?example-group=access-control) if you're not sure where to start.
- Install the [VS Code extension](https://marketplace.visualstudio.com/items?itemName=tsandall.opa) to get started locally with live diagnostics, debugging and formatting. See [Editor and IDE Support](https://www.openpolicyagent.org/docs/editor-and-ide-support) for other supported editors.
- Go to the [OPA Documentation](https://www.openpolicyagent.org/docs) to
  learn about the Rego language as well as how to deploy and integrate OPA.
- Check out the learning resources in the [Learning Rego](https://www.openpolicyagent.org/ecosystem/by-feature/learning-rego) section of the ecosystem directory.
- Follow the [Running OPA](https://www.openpolicyagent.org/docs/latest/#running-opa) instructions to get started with the OPA CLI locally.
- See [Docker Hub](https://hub.docker.com/r/openpolicyagent/opa/tags/) for container images and the [GitHub releases](https://github.com/open-policy-agent/opa/releases) for binaries.
- Check out the [OPA Roadmap](https://github.com/orgs/open-policy-agent/projects/10) to see a high-level snapshot of OPA features in-progress and planned.

## Want to talk about OPA or get support?

- Join the [OPA Slack](https://slack.openpolicyagent.org) to talk to other OPA users and maintainers. See `#help` for support.
- Check out the [Community Discussions](https://github.com/orgs/open-policy-agent/discussions) to ask questions.
- See the [Support](https://www.openpolicyagent.org/support) page for commercial support options.

## Interested to learn what others are doing with OPA?

- Browse community projects on the [OPA Ecosystem Directory](https://www.openpolicyagent.org/ecosystem) - don't forget to [list your own](https://github.com/open-policy-agent/opa/tree/main/docs#opa-ecosystem)!
- Check out the [ADOPTERS.md](./ADOPTERS.md) file for a list of production adopters. Does your organization use OPA in production? Support the OPA project by submitting a PR to add your organization to the list with a short description of your OPA use cases!

## Want to integrate OPA?

- See the high-level [Go SDK](https://www.openpolicyagent.org/docs/integration#integrating-with-the-go-sdk) or the low-level Go API
  [![GoDoc](https://pkg.go.dev/badge/github.com/open-policy-agent/opa?utm_source=godoc)](https://pkg.go.dev/github.com/open-policy-agent/opa/rego?utm_source=godoc)
  to integrate OPA with services written in Go.
- See the [REST API](https://www.openpolicyagent.org/docs/rest-api.html)
  reference to integrate OPA with services written in other languages.
- See the [integration docs](https://www.openpolicyagent.org/docs/integration) for more options.

## Want to contribute to OPA?

- Read the [Contributing Guide](https://www.openpolicyagent.org/docs/contributing) to learn how to make your first contribution.
- Use [#contributors](https://openpolicyagent.slack.com/?redir=%2Farchives%2FC02L1TLPN59%3Fname%3DC02L1TLPN59) in Slack to talk to other contributors and OPA maintainers.
- File a [GitHub Issue](https://github.com/open-policy-agent/opa/issues) to request features or report bugs.

## How does OPA work?

OPA gives you a high-level declarative language to author and enforce policies
across your stack.


