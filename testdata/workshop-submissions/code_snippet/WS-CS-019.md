# Terraform AWS VPC simple example [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/terraform-aws-modules/terraform-aws-vpc/blob/master/examples/simple/main.tf
  repo: terraform-aws-modules/terraform-aws-vpc
  path: examples/simple/main.tf
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 3b89db922034
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: iac, aws, networking.

### File: vpc.tf

```hcl
provider "aws" {
  region = local.region
}

data "aws_availability_zones" "available" {}

locals {
  name   = "ex-${basename(path.cwd)}"
  region = "eu-west-1"

  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)

  tags = {
    Example    = local.name
    GithubRepo = "terraform-aws-vpc"
    GithubOrg  = "terraform-aws-modules"
  }
}

################################################################################
# VPC Module
################################################################################

module "vpc" {
  source = "../../"

  name = local.name
  cidr = local.vpc_cidr

  azs             = local.azs
  private_subnets = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 4, k)]

  tags = local.tags
}

################################################################################
# Disabled
################################################################################

module "disabled" {
  source = "../../"

  create_vpc = false
}
```
