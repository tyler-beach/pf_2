resource "aws_security_group" "open_ssh" {
  name        = "open-ssh"
  description = "intentionally open for fixture"
  vpc_id      = "vpc-12345678"

  ingress {
    description = "SSH from the world"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "open_rdp" {
  name   = "open-rdp"
  vpc_id = "vpc-12345678"

  ingress {
    from_port   = 3389
    to_port     = 3389
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_iam_policy" "admin_star" {
  name = "admin-star"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "*"
      Resource = "*"
    }]
  })
}

resource "aws_instance" "imds_v1" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "optional"
    http_put_response_hop_limit = 2
  }

  root_block_device {
    encrypted = false
  }
}

resource "aws_iam_account_password_policy" "weak" {
  minimum_password_length        = 8
  require_lowercase_characters   = false
  require_numbers                = false
  require_uppercase_characters   = false
  require_symbols                = false
  allow_users_to_change_password = true
}

resource "aws_cloudwatch_log_group" "no_retention" {
  name = "app-logs"
}

resource "aws_s3_bucket" "mutable_logs" {
  bucket = "mutable-log-archive"
}

resource "aws_cloudtrail" "no_sns" {
  name                          = "org-trail"
  s3_bucket_name                = "trail-bucket"
  include_global_service_events = false
  is_multi_region_trail         = false
  enable_log_file_validation    = false
}
