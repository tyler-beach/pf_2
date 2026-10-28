# OPA Gatekeeper required-labels template [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/open-policy-agent/gatekeeper/blob/master/demo/basic/templates/k8srequiredlabels_template.yaml
  repo: open-policy-agent/gatekeeper
  path: demo/basic/templates/k8srequiredlabels_template.yaml
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: ed4d53be07a4
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: policy, k8s.

### File: k8srequiredlabels_template.yaml

```yaml
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: k8srequiredlabels
spec:
  crd:
    spec:
      names:
        kind: K8sRequiredLabels
      validation:
        # Schema for the `parameters` field
        openAPIV3Schema:
          type: object
          properties:
            labels:
              type: array
              items:
                type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredlabels

        violation[{"msg": msg, "details": {"missing_labels": missing}}] {
          provided := {label | input.review.object.metadata.labels[label]}
          required := {label | label := input.parameters.labels[_]}
          missing := required - provided
          count(missing) > 0
          msg := sprintf("you must provide labels: %v", [missing])
        }
```
