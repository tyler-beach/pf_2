# Terraform S3 bucket module fragment [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/terraform-aws-modules/terraform-aws-s3-bucket/blob/master/main.tf
  repo: terraform-aws-modules/terraform-aws-s3-bucket
  path: main.tf
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 3a457bf824d2
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: iac, aws, data.

### File: s3.tf

```hcl
data "aws_region" "current" {
  count = local.create_bucket ? 1 : 0

  region = var.region
}

data "aws_canonical_user_id" "this" {
  count = local.create_bucket && local.create_bucket_acl && try(var.owner["id"], null) == null ? 1 : 0
}

data "aws_caller_identity" "current" {
  count = local.create_bucket ? 1 : 0
}

data "aws_partition" "current" {
  count = local.create_bucket ? 1 : 0
}

locals {
  account_id = try(data.aws_caller_identity.current[0].account_id, "")
  partition  = try(data.aws_partition.current[0].partition, "")
  region     = try(data.aws_region.current[0].region, "")
}

locals {
  create_bucket = var.create_bucket && var.putin_khuylo

  create_bucket_acl = (var.acl != null && var.acl != "null") || length(local.grants) > 0

  attach_policy = var.attach_require_latest_tls_policy || var.attach_access_log_delivery_policy || var.attach_elb_log_delivery_policy || var.attach_lb_log_delivery_policy || var.attach_cloudtrail_log_delivery_policy || var.attach_deny_insecure_transport_policy || var.attach_inventory_destination_policy || var.attach_analytics_destination_policy || var.attach_deny_incorrect_encryption_headers || var.attach_deny_incorrect_kms_key_sse || var.attach_deny_unencrypted_object_uploads || var.attach_deny_ssec_encrypted_object_uploads || var.attach_policy || var.attach_waf_log_delivery_policy

  # Placeholders in the policy document to be replaced with the actual values
  policy_placeholders = {
    "_S3_BUCKET_ID_"   = try(var.is_directory_bucket ? aws_s3_directory_bucket.this[0].bucket : aws_s3_bucket.this[0].id, null),
    "_S3_BUCKET_ARN_"  = try(var.is_directory_bucket ? aws_s3_directory_bucket.this[0].arn : aws_s3_bucket.this[0].arn, null),
    "_AWS_ACCOUNT_ID_" = local.account_id
  }

  policy = local.create_bucket && local.attach_policy ? replace(
    replace(
```
