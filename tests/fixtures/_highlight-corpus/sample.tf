# Terraform / HCL sample
terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "tags" {
  type = map(string)
  default = {
    Project = "spy"
    Env     = "dev"
  }
}

provider "aws" {
  region = var.region
}

resource "aws_s3_bucket" "logs" {
  bucket = "spy-logs-${terraform.workspace}"
  tags   = var.tags
}

resource "aws_s3_bucket_versioning" "logs" {
  bucket = aws_s3_bucket.logs.id
  versioning_configuration {
    status = "Enabled"
  }
}

output "bucket_name" {
  value = aws_s3_bucket.logs.bucket
}
