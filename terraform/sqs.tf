# ── Dead-letter queues ─────────────────────────────────────────────────────────

resource "aws_sqs_queue" "request_dlq" {
  name                      = "${local.name_prefix}-request-dlq"
  message_retention_seconds = 1209600 # 14 days — keep failed messages longer for investigation
}

resource "aws_sqs_queue" "response_dlq" {
  name                      = "${local.name_prefix}-response-dlq"
  message_retention_seconds = 1209600
}

# ── Request queue ──────────────────────────────────────────────────────────────
# awspim sends approval requests here; awspim-manager polls and processes them.

resource "aws_sqs_queue" "request" {
  name                       = "${local.name_prefix}-request"
  visibility_timeout_seconds = var.sqs_visibility_timeout_seconds
  message_retention_seconds  = var.sqs_message_retention_seconds

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.request_dlq.arn
    maxReceiveCount     = 3
  })
}

# ── Response queue ─────────────────────────────────────────────────────────────
# awspim-manager sends results here; awspim polls and DMs users with outcomes.

resource "aws_sqs_queue" "response" {
  name                       = "${local.name_prefix}-response"
  visibility_timeout_seconds = var.sqs_visibility_timeout_seconds
  message_retention_seconds  = var.sqs_message_retention_seconds

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.response_dlq.arn
    maxReceiveCount     = 3
  })
}
