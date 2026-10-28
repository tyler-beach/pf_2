# Terraform EKS auto-mode example [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/terraform-aws-modules/terraform-aws-eks/blob/master/examples/eks-auto-mode/main.tf
  repo: terraform-aws-modules/terraform-aws-eks
  path: examples/eks-auto-mode/main.tf
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: d20da6811c8f
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: iac, aws, k8s.

### File: eks.tf

```hcl
provider "aws" {
  region = local.region
}

data "aws_availability_zones" "available" {
  # Exclude local zones
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

locals {
  name               = "ex-${basename(path.cwd)}"
  kubernetes_version = "1.33"
  region             = "us-west-2"

  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)

  tags = {
    Test       = local.name
    GithubRepo = "terraform-aws-eks"
    GithubOrg  = "terraform-aws-modules"
  }
}

################################################################################
# EKS Module
################################################################################

module "eks" {
  source = "../.."

  name                   = local.name
  kubernetes_version     = local.kubernetes_version
  endpoint_public_access = true

  enable_cluster_creator_admin_permissions = true

  compute_config = {
    enabled    = true
    node_pools = ["general-purpose"]
  }

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  tags = local.tags
}

module "eks_auto_custom_node_pools" {
  source = "../.."

  name                   = "${local.name}-custom"
```
