# Checkov SG unrestricted ingress22 fixture [code_snippet]

<!-- submission_kind: code_snippet -->
<!-- provenance:
  source_url: https://github.com/bridgecrewio/checkov/blob/main/tests/terraform/checks/resource/aws/example_SecurityGroupUnrestrictedIngress22/main.tf
  repo: bridgecrewio/checkov
  path: tests/terraform/checks/resource/aws/example_SecurityGroupUnrestrictedIngress22/main.tf
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 674cd55b3fea
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **code_snippet**. Domains: iac, aws, security.

### File: sg_open.tf

```hcl
resource "aws_security_group" "fail" {
  name   = "sg-bar"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port = 22
    to_port   = 22
    protocol  = "tcp"
    cidr_blocks = ["192.168.0.0/16", "0.0.0.0/0"]
    description = "foo"
  }

  egress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "fail2" {
  name   = "sg-bar"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "tcp"
    cidr_blocks = ["192.168.0.0/16", "0.0.0.0/0"]
    description = "foo"
  }

  egress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "fail3" {
  name   = "sg-bar"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port = 22
    to_port   = 22
    protocol  = "tcp"
    security_groups = [aws_security_group.foo-sg.id]
    cidr_blocks = ["192.168.0.0/16", "0.0.0.0/0"]
    description = "foo"
  }

  egress {
    from_port = 0
```
