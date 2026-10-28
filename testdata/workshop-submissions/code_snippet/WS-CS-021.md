# Traefik BasicAuth middleware docs/config [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/traefik/traefik/blob/v3.1/docs/content/middlewares/http/basicauth.md
  repo: traefik/traefik
  path: docs/content/middlewares/http/basicauth.md
  license: MIT
  retrieved: 2026-10-28
  excerpt_sha256_12: 5f74dcac717b
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: auth, networking, apis.

### File: basicauth.md

```markdown
---
title: "Traefik BasicAuth Documentation"
description: "The HTTP basic authentication (BasicAuth) middleware in Traefik Proxy restricts access to your Services to known users. Read the technical documentation."
---

# BasicAuth

Adding Basic Authentication
{: .subtitle }

![BasicAuth](../../assets/img/middleware/basicauth.png)

The BasicAuth middleware grants access to services to authorized users only.

## Configuration Examples

```yaml tab="Docker & Swarm"
# Declaring the user list
#
# Note: when used in docker-compose.yml all dollar signs in the hash need to be doubled for escaping.
# To create user:password pair, it's possible to use this command:
# echo $(htpasswd -nB user) | sed -e s/\\$/\\$\\$/g
#
# Also note that dollar signs should NOT be doubled when they not evaluated (e.g. Ansible docker_container module).
labels:
  - "traefik.http.middlewares.test-auth.basicauth.users=test:$$apr1$$H6uskkkW$$IgXLP6ewTrSuBkTrqE8wj/,test2:$$apr1$$d9hr9HBB$$4HxwgUir3HP4EsggP/QNo0"
```

```yaml tab="Kubernetes"
# Declaring the user list
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: test-auth
spec:
  basicAuth:
    secret: secretName
```

```yaml tab="Consul Catalog"
- "traefik.http.middlewares.test-auth.basicauth.users=test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/,test2:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0"
```

```yaml tab="File (YAML)"
# Declaring the user list
http:
  middlewares:
    test-auth:
      basicAuth:
        users:
          - "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"
          - "test2:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0"
```

```toml tab="File (TOML)"
```
