# Kubernetes nginx Deployment example [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/kubernetes/website/blob/main/content/en/examples/controllers/nginx-deployment.yaml
  repo: kubernetes/website
  path: content/en/examples/controllers/nginx-deployment.yaml
  license: CC-BY-4.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 0c5142924363
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: k8s, reliability.

### File: nginx-deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
```
