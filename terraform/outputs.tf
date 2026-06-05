# ── SQS ────────────────────────────────────────────────────────────────────────

output "request_queue_url" {
  description = "Set as manager_sqs_arn in awspim config"
  value       = aws_sqs_queue.request.url
}

output "response_queue_url" {
  description = "Set as response_sqs_url in awspim config"
  value       = aws_sqs_queue.response.url
}

output "request_queue_arn" {
  description = "ARN of the request SQS queue (for awspim-manager trigger)"
  value       = aws_sqs_queue.request.arn
}

# ── Secrets Manager ────────────────────────────────────────────────────────────

output "aws_accounts_secret_arn" {
  description = "ARN of the AWS accounts secret"
  value       = aws_secretsmanager_secret.aws_accounts.arn
}

output "aws_accounts_secret_name" {
  description = "Set as aws_accounts_secret in awspim config"
  value       = aws_secretsmanager_secret.aws_accounts.name
}

output "mfa_registrations_secret_arn" {
  description = "ARN of the MFA registrations secret"
  value       = aws_secretsmanager_secret.mfa_registrations.arn
}

output "mfa_registrations_secret_name" {
  description = "Set as mfa_storage_secret in awspim config"
  value       = aws_secretsmanager_secret.mfa_registrations.name
}

# ── IAM ────────────────────────────────────────────────────────────────────────

output "awspim_role_arn" {
  description = "IAM role ARN for the awspim bot — attach to the instance/task/pod running awspim"
  value       = aws_iam_role.awspim.arn
}

output "manager_role_arn" {
  description = "IAM role ARN for awspim-manager Lambda — use in the manager Terraform config"
  value       = aws_iam_role.manager.arn
}

# ── DynamoDB ───────────────────────────────────────────────────────────────────

output "dynamodb_table_name" {
  description = "Set as DYNAMO_TABLE env var in the awspim-manager Lambda"
  value       = aws_dynamodb_table.pim_requests.name
}

# ── Approvers secret ───────────────────────────────────────────────────────────

output "approvers_secret_name" {
  description = "Set as APPROVERS env var in the awspim-manager Lambda"
  value       = aws_secretsmanager_secret.approvers.name
}

# ── Lambda ─────────────────────────────────────────────────────────────────────

output "manager_lambda_name" {
  description = "awspim-manager Lambda function name"
  value       = aws_lambda_function.manager.function_name
}

# ── awspim config snippet ──────────────────────────────────────────────────────

output "awspim_config_snippet" {
  description = "Ready-to-use config.yaml values — paste into your awspim config"
  value       = <<-EOT
    aws_region: "${var.aws_region}"
    aws_accounts_secret: "${aws_secretsmanager_secret.aws_accounts.name}"
    mfa_storage_secret: "${aws_secretsmanager_secret.mfa_registrations.name}"
    manager_sqs_arn: "${aws_sqs_queue.request.url}"
    response_sqs_url: "${aws_sqs_queue.response.url}"
  EOT
}
