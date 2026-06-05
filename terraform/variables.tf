variable "aws_region" {
  description = "AWS region to deploy resources into"
  type        = string
  default     = "us-east-2"
}

variable "environment" {
  description = "Environment name (e.g. prod, staging, dev)"
  type        = string
  default     = "prod"
}

variable "prefix" {
  description = "Resource name prefix"
  type        = string
  default     = "pim"
}

# ── SQS ────────────────────────────────────────────────────────────────────────

variable "sqs_message_retention_seconds" {
  description = "How long SQS retains unprocessed messages (seconds)"
  type        = number
  default     = 86400 # 1 day
}

variable "sqs_visibility_timeout_seconds" {
  description = "SQS visibility timeout (seconds) — should be >= Lambda timeout"
  type        = number
  default     = 60
}

# ── SES ────────────────────────────────────────────────────────────────────────

variable "ses_from_email" {
  description = "Verified SES sender email address (must be verified in SES)"
  type        = string
}

variable "mfa_email_template_name" {
  description = "SES template name for MFA verification emails"
  type        = string
  default     = "PIM-MFA-Template"
}

# ── Lambda / awspim-manager ────────────────────────────────────────────────────

variable "manager_lambda_zip_path" {
  description = "Local path to the awspim-manager zip file (e.g. ../awspim-manager/dist/awspim-manager.zip)"
  type        = string
}

variable "lambda_timeout_seconds" {
  description = "Lambda function timeout in seconds (should be <= SQS visibility timeout)"
  type        = number
  default     = 30
}

variable "session_timeout_seconds" {
  description = "How long (seconds) a granted PIM session lasts before auto-revocation"
  type        = number
  default     = 3600 # 1 hour
}

variable "pim_permission_set_name" {
  description = "AWS Identity Center permission set name (or ARN) to assign to requestors"
  type        = string
  default     = "AdministratorAccess"
}

variable "management_account_id" {
  description = "12-digit AWS account ID of the management/org account — requests targeting this account are blocked"
  type        = string
}

# ── Tags ───────────────────────────────────────────────────────────────────────

variable "tags" {
  description = "Common tags applied to all resources"
  type        = map(string)
  default     = {}
}
