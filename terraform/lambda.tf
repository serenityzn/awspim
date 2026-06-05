# ── awspim-manager Lambda ──────────────────────────────────────────────────────
# Processes approval requests from the SQS request queue and revokes expired
# sessions on a schedule. See: https://github.com/serenityzn/awspim-manager

resource "aws_lambda_function" "manager" {
  function_name = "${local.name_prefix}-manager"
  description   = "awspim-manager: processes PIM access requests and revokes expired sessions"
  role          = aws_iam_role.manager.arn

  package_type     = "Zip"
  filename         = var.manager_lambda_zip_path
  source_code_hash = filebase64sha256(var.manager_lambda_zip_path)
  handler          = "bootstrap"
  runtime          = "provided.al2023"

  architectures = ["x86_64"]
  timeout       = var.lambda_timeout_seconds
  memory_size   = 256

  environment {
    variables = {
      AWS_REGION           = var.aws_region
      DYNAMO_TABLE         = aws_dynamodb_table.pim_requests.name
      APPROVERS            = aws_secretsmanager_secret.approvers.name
      SES_FROM_EMAIL       = var.ses_from_email
      SQS_RESPONSE_QUEUE_URL = aws_sqs_queue.response.url
      MANAGEMENT_ACCOUNT   = var.management_account_id
      SESSION_TIMEOUT      = tostring(var.session_timeout_seconds)
      PIM_ROLE             = var.pim_permission_set_name
      LOG_LEVEL            = "info"
    }
  }
}

# ── SQS event source mapping ───────────────────────────────────────────────────
# Triggers Lambda for each batch of approval messages from the request queue.

resource "aws_lambda_event_source_mapping" "manager_sqs" {
  event_source_arn = aws_sqs_queue.request.arn
  function_name    = aws_lambda_function.manager.arn
  batch_size       = 1 # process one approval at a time

  function_response_types = ["ReportBatchItemFailures"]
}

# ── EventBridge scheduled cleanup ─────────────────────────────────────────────
# Triggers the Lambda every 15 minutes to revoke expired sessions.

resource "aws_cloudwatch_event_rule" "cleanup" {
  name                = "${local.name_prefix}-cleanup-schedule"
  description         = "Triggers awspim-manager to revoke expired PIM sessions"
  schedule_expression = "rate(15 minutes)"
}

resource "aws_cloudwatch_event_target" "cleanup" {
  rule      = aws_cloudwatch_event_rule.cleanup.name
  target_id = "awspim-manager-cleanup"
  arn       = aws_lambda_function.manager.arn
}

resource "aws_lambda_permission" "allow_eventbridge" {
  statement_id  = "AllowEventBridgeInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.manager.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.cleanup.arn
}

# ── CloudWatch Log Group ───────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "manager" {
  name              = "/aws/lambda/${aws_lambda_function.manager.function_name}"
  retention_in_days = 30
}
