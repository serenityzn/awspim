# ── ECR Repositories ───────────────────────────────────────────────────────────

locals {
  ecr_lifecycle_policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last ${var.ecr_image_retention_count} images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = var.ecr_image_retention_count
      }
      action = { type = "expire" }
    }]
  })
}

# awspim — Slack bot (https://github.com/serenityzn/awspim)
resource "aws_ecr_repository" "awspim" {
  name                 = "awspim"
  image_tag_mutability = "MUTABLE"
  image_scanning_configuration { scan_on_push = true }
}

resource "aws_ecr_lifecycle_policy" "awspim" {
  repository = aws_ecr_repository.awspim.name
  policy     = local.ecr_lifecycle_policy
}

