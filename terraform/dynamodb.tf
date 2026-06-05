# ── PIM Sessions table ─────────────────────────────────────────────────────────
# Tracks active and expired PIM access sessions.
# awspim-manager writes session records here and scans for expired ones.

resource "aws_dynamodb_table" "pim_requests" {
  name         = "${local.name_prefix}-requests"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "request_id"
  range_key    = "created_timestamp"

  attribute {
    name = "request_id"
    type = "S"
  }

  attribute {
    name = "created_timestamp"
    type = "N"
  }

  attribute {
    name = "status"
    type = "S"
  }

  attribute {
    name = "expiration_timestamp"
    type = "N"
  }

  # GSI used by the cleanup job to find expired sessions efficiently
  global_secondary_index {
    name            = "status-expiration-index"
    hash_key        = "status"
    range_key       = "expiration_timestamp"
    projection_type = "ALL"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }
}
