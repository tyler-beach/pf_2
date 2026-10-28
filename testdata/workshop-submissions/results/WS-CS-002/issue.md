# Terraform IAM policy module [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/terraform-aws-modules/terraform-aws-iam/blob/master/modules/iam-policy/main.tf
  repo: terraform-aws-modules/terraform-aws-iam
  path: modules/iam-policy/main.tf
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 074cac22c2bf
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: iac, aws, auth, iam.

### File: iam-policy.tf

```hcl
################################################################################
# IAM Policy
################################################################################

resource "aws_iam_policy" "policy" {
  count = var.create ? 1 : 0

  description = var.description
  name        = var.name
  name_prefix = var.name_prefix
  path        = var.path
  policy      = var.policy

  tags = var.tags
}
```
