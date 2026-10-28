# Kubernetes Ingress example [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/kubernetes/website/blob/main/content/en/examples/service/networking/example-ingress.yaml
  repo: kubernetes/website
  path: content/en/examples/service/networking/example-ingress.yaml
  license: CC-BY-4.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 6ca7755a08da
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: k8s, networking.

### File: example-ingress.yaml

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-ingress
spec:
  ingressClassName: nginx
  rules:
    - host: hello-world.example
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 8080
```
