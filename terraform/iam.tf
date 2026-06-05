# ── awspim IAM role ────────────────────────────────────────────────────────────
# Attach this role to the EC2 instance / ECS task / pod running awspim.

data "aws_iam_policy_document" "awspim_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "awspim" {
  name               = "${local.name_prefix}-bot"
  assume_role_policy = data.aws_iam_policy_document.awspim_assume_role.json
  description        = "Role used by the awspim Slack bot"
}

data "aws_iam_policy_document" "awspim_permissions" {
  # Secrets Manager — read accounts list and MFA registrations
  statement {
    sid    = "SecretsManagerRead"
    effect = "Allow"
    actions = [
      "secretsmanager:GetSecretValue",
      "secretsmanager:PutSecretValue", # needed to write TOTP registrations
      "secretsmanager:UpdateSecret",
    ]
    resources = [
      aws_secretsmanager_secret.aws_accounts.arn,
      aws_secretsmanager_secret.mfa_registrations.arn,
    ]
  }

  # SQS request queue — send approval requests to awspim-manager
  statement {
    sid    = "SQSRequestQueueSend"
    effect = "Allow"
    actions = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.request.arn]
  }

  # SQS response queue — poll results sent back by awspim-manager
  statement {
    sid    = "SQSResponseQueueReceive"
    effect = "Allow"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.response.arn]
  }

  # SES — send MFA verification emails
  statement {
    sid    = "SESSendEmail"
    effect = "Allow"
    actions = [
      "ses:SendEmail",
      "ses:SendTemplatedEmail",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "ses:FromAddress"
      values   = [var.ses_from_email]
    }
  }
}

resource "aws_iam_policy" "awspim" {
  name        = "${local.name_prefix}-bot-policy"
  description = "Permissions for the awspim Slack bot"
  policy      = data.aws_iam_policy_document.awspim_permissions.json
}

resource "aws_iam_role_policy_attachment" "awspim" {
  role       = aws_iam_role.awspim.name
  policy_arn = aws_iam_policy.awspim.arn
}

# ── awspim-manager Lambda IAM role ─────────────────────────────────────────────
# See: https://github.com/serenityzn/awspim-manager
# This role is referenced here so the SQS queue policies can grant it access.
# The full Lambda role definition lives in the awspim-manager Terraform config.

resource "aws_iam_role" "manager" {
  name               = "${local.name_prefix}-manager"
  assume_role_policy = data.aws_iam_policy_document.manager_assume_role.json
  description        = "Role used by the awspim-manager Lambda function"
}

data "aws_iam_policy_document" "manager_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "manager_permissions" {
  # CloudWatch Logs
  statement {
    sid    = "CloudWatchLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["arn:aws:logs:*:*:*"]
  }

  # SQS request queue — receive and delete messages
  statement {
    sid    = "SQSRequestQueueReceive"
    effect = "Allow"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.request.arn]
  }

  # SQS response queue — send results back to awspim
  statement {
    sid     = "SQSResponseQueueSend"
    effect  = "Allow"
    actions = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.response.arn]
  }

  # DynamoDB — track active and expired PIM sessions
  statement {
    sid    = "DynamoDB"
    effect = "Allow"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:Query",
      "dynamodb:Scan",
      "dynamodb:DescribeTable",
    ]
    resources = [
      aws_dynamodb_table.pim_requests.arn,
      "${aws_dynamodb_table.pim_requests.arn}/index/*",
    ]
  }

  # Secrets Manager — read approver allowlist
  statement {
    sid     = "SecretsManagerRead"
    effect  = "Allow"
    actions = ["secretsmanager:GetSecretValue"]
    resources = [aws_secretsmanager_secret.approvers.arn]
  }

  # SES — send approval/rejection/expiry emails
  statement {
    sid     = "SESSendEmail"
    effect  = "Allow"
    actions = ["ses:SendEmail"]
    resources = ["*"]
  }

  # AWS Identity Center — assign/revoke permission sets
  # Uses sso-admin:* (not legacy sso:*) as per live code in identitycenter.go
  statement {
    sid    = "IdentityCenter"
    effect = "Allow"
    actions = [
      "sso-admin:ListInstances",
      "sso-admin:ListPermissionSets",
      "sso-admin:DescribePermissionSet",
      "sso-admin:CreateAccountAssignment",
      "sso-admin:DeleteAccountAssignment",
      "sso-admin:ListAccountAssignments",
      "sso-admin:DescribeAccountAssignmentCreationStatus",
      "sso-admin:DescribeAccountAssignmentDeletionStatus",
      "identitystore:ListUsers",
      "identitystore:DescribeUser",
      "organizations:DescribeAccount",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "manager" {
  name        = "${local.name_prefix}-manager-policy"
  description = "Permissions for the awspim-manager Lambda function"
  policy      = data.aws_iam_policy_document.manager_permissions.json
}

resource "aws_iam_role_policy_attachment" "manager" {
  role       = aws_iam_role.manager.name
  policy_arn = aws_iam_policy.manager.arn
}
