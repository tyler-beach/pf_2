# ZooKeeper YAML (incl. PDB patterns) [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/kubernetes/website/blob/main/content/en/examples/application/zookeeper/zookeeper.yaml
  repo: kubernetes/website
  path: content/en/examples/application/zookeeper/zookeeper.yaml
  license: CC-BY-4.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 829f5d68473f
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: k8s, reliability.

### File: zookeeper.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: zk-hs
  labels:
    app: zk
spec:
  ports:
  - port: 2888
    name: server
  - port: 3888
    name: leader-election
  clusterIP: None
  selector:
    app: zk
---
apiVersion: v1
kind: Service
metadata:
  name: zk-cs
  labels:
    app: zk
spec:
  ports:
  - port: 2181
    name: client
  selector:
    app: zk
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: zk-pdb
spec:
  selector:
    matchLabels:
      app: zk
  maxUnavailable: 1
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: zk
spec:
  selector:
    matchLabels:
      app: zk
  serviceName: zk-hs
  replicas: 3
  updateStrategy:
    type: RollingUpdate
  podManagementPolicy: OrderedReady
  template:
    metadata:
      labels:
```
