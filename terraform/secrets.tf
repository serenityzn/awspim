# ── AWS Accounts secret ────────────────────────────────────────────────────────
# Stores the list of AWS accounts users can request access to.
# Populate after creation:
#   aws secretsmanager put-secret-value \
#     --secret-id <arn> \
#     --secret-string '{"accounts":[{"accountid":"123456789012","accountname":"Production"}]}'

resource "aws_secretsmanager_secret" "aws_accounts" {
  name        = "${local.name_prefix}/aws-accounts"
  description = "List of AWS accounts available for PIM access requests"
}

resource "aws_secretsmanager_secret_version" "aws_accounts_initial" {
  secret_id = aws_secretsmanager_secret.aws_accounts.id
  secret_string = jsonencode({
    accounts = []
  })

  lifecycle {
    # Prevent Terraform from overwriting the secret value after initial creation
    ignore_changes = [secret_string]
  }
}

# ── Approvers secret ───────────────────────────────────────────────────────────
# Used by awspim-manager to validate who is allowed to approve requests.
# Populate after creation:
#   aws secretsmanager put-secret-value \
#     --secret-id <arn> \
#     --secret-string '["approver@yourdomain.com","manager@yourdomain.com"]'

resource "aws_secretsmanager_secret" "approvers" {
  name        = "${local.name_prefix}/approvers"
  description = "List of authorized approver email addresses for PIM requests"
}

resource "aws_secretsmanager_secret_version" "approvers_initial" {
  secret_id     = aws_secretsmanager_secret.approvers.id
  secret_string = jsonencode([])

  lifecycle {
    ignore_changes = [secret_string]
  }
}

# ── MFA registrations secret ───────────────────────────────────────────────────
# awspim writes TOTP keys here when users run /register-totp.
# Created empty — managed entirely by the application at runtime.

resource "aws_secretsmanager_secret" "mfa_registrations" {
  name        = "${local.name_prefix}/mfa-registrations"
  description = "TOTP registration data for PIM approvers (managed by awspim at runtime)"
}

resource "aws_secretsmanager_secret_version" "mfa_registrations_initial" {
  secret_id     = aws_secretsmanager_secret.mfa_registrations.id
  secret_string = jsonencode({})

  lifecycle {
    ignore_changes = [secret_string]
  }
}
