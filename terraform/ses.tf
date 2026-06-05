# ── SES Email Template ─────────────────────────────────────────────────────────
# Used by awspim to send MFA verification codes to approvers.
# NOTE: var.ses_from_email must be verified in SES before emails can be sent.

resource "aws_ses_template" "mfa" {
  name    = var.mfa_email_template_name
  subject = "AWS PIM - Multi-Factor Authentication Required"

  html = <<-HTML
    <!DOCTYPE html>
    <html>
    <head><meta charset="utf-8"><title>AWS PIM - MFA</title></head>
    <body style="font-family:Arial,sans-serif;max-width:600px;margin:0 auto;padding:20px;">
      <div style="background:#f8f9fa;padding:20px;border-radius:8px;margin-bottom:20px;">
        <h1 style="color:#333;margin:0;font-size:24px;">🔐 AWS PIM - Multi-Factor Authentication</h1>
      </div>
      <div style="background:#fff;padding:20px;border:1px solid #dee2e6;border-radius:8px;">
        <h2 style="color:#495057;margin-top:0;">Approval Verification Required</h2>
        <p>Hello,</p>
        <p>You are attempting to approve an AWS access request. Please use the verification code below:</p>
        <div style="background:#e9ecef;padding:15px;border-radius:4px;text-align:center;margin:20px 0;">
          <strong style="font-size:24px;color:#495057;letter-spacing:2px;">{{code}}</strong>
        </div>
        <div style="background:#f8f9fa;padding:15px;border-left:4px solid #007bff;margin:20px 0;">
          <h3 style="margin:0 0 10px 0;color:#495057;">Request Details:</h3>
          <p style="margin:5px 0;"><strong>Requestor:</strong> {{requestor}}</p>
          <p style="margin:5px 0;"><strong>Account:</strong> {{account_name}} ({{account_id}})</p>
        </div>
        <p><strong>Important:</strong> This code expires in 10 minutes.</p>
        <p>Enter this code along with your TOTP code in the Slack verification form.</p>
      </div>
      <div style="text-align:center;margin-top:20px;color:#6c757d;font-size:12px;">
        <p>This is an automated message from AWS PIM Management System</p>
      </div>
    </body>
    </html>
  HTML

  text = <<-TEXT
    AWS PIM - Multi-Factor Authentication Required

    Hello,

    You are attempting to approve an AWS access request.
    Please use the verification code below:

    Code: {{code}}

    Request Details:
    - Requestor: {{requestor}}
    - Account: {{account_name}} ({{account_id}})

    This code expires in 10 minutes.
    Enter this code along with your TOTP code in the Slack verification form.

    ---
    This is an automated message from AWS PIM Management System
  TEXT
}
