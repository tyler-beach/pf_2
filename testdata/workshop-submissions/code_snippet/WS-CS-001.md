# Terraform AWS security group example [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/terraform-aws-modules/terraform-aws-security-group/blob/master/examples/complete/main.tf
  repo: terraform-aws-modules/terraform-aws-security-group
  path: examples/complete/main.tf
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 8b31fe3d4c72
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: iac, aws, networking.

### File: main.tf

```hcl
provider "aws" {
  region = local.region
}

data "aws_availability_zones" "available" {
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

locals {
  region   = "eu-west-1"
  name     = "ex-${basename(path.cwd)}"
  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)

  tags = {
    Name       = local.name
    Example    = local.name
    Repository = "https://github.com/terraform-aws-modules/terraform-aws-security-group"
  }
}

################################################################################
# Security Group
################################################################################

module "security_group" {
  source = "../../"

  name        = local.name
  description = "Complete security group example"
  vpc_id      = module.vpc.vpc_id

  ingress_rules = {
    https-from-vpc = {
      from_port   = 443
      to_port     = 443
      ip_protocol = "tcp"
      cidr_ipv4   = local.vpc_cidr
      description = "HTTPS from VPC"
    }

    http-from-ipv6 = {
      from_port   = 80
      to_port     = 80
      ip_protocol = "tcp"
      cidr_ipv6   = "2001:db8::/64"
      description = "HTTP from IPv6"
    }

    all-from-self = {
      ip_protocol                  = "-1"
      referenced_security_group_id = "self"
```
