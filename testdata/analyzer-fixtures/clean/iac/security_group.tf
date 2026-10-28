resource "aws_security_group" "restricted_ssh" {
  name        = "restricted-ssh"
  description = "SSH limited to bastion CIDR"
  vpc_id      = "vpc-12345678"

  ingress {
    description = "SSH from bastion"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/8"]
  }

  egress {
    description = "HTTPS egress"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_iam_policy" "least_privilege" {
  name = "least-privilege-read"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::example-bucket/*"]
    }]
  })
}

resource "aws_instance" "imds_v2" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  root_block_device {
    encrypted = true
  }
}

resource "aws_flow_log" "vpc" {
  vpc_id          = "vpc-12345678"
  traffic_type    = "ALL"
  iam_role_arn    = "arn:aws:iam::123456789012:role/flowlogs"
  log_destination = "arn:aws:logs:us-east-1:123456789012:log-group:vpc-flow"
}

resource "aws_iam_account_password_policy" "strong" {
  minimum_password_length        = 14
  require_lowercase_characters   = true
  require_numbers                = true
  require_uppercase_characters   = true
  require_symbols                = true
  allow_users_to_change_password = true
  password_reuse_prevention      = 24
  max_password_age               = 90
}

resource "aws_cloudwatch_log_group" "retained" {
  name              = "app-logs"
  retention_in_days = 365
}

resource "aws_s3_bucket" "immutable_logs" {
  bucket              = "immutable-log-archive"
  object_lock_enabled = true
}

resource "aws_cloudtrail" "with_sns" {
  name                          = "org-trail"
  s3_bucket_name                = "trail-bucket"
  include_global_service_events = true
  is_multi_region_trail         = true
  enable_log_file_validation    = true
  sns_topic_name                = "trail-alerts"
}
