# Atlas configuration for the versioned migration workflow.
#
# Two URLs matter:
#   url  - the database migrations are applied to (from DATABASE_URL)
#   dev  - a scratch database Atlas uses to compute and validate diffs. It is
#          wiped on every run, so it must never point at real data.

variable "database_url" {
  type        = string
  default     = getenv("DATABASE_URL")
  description = "Target database. Taken from DATABASE_URL (see .env)."
}

variable "revisions_schema" {
  type        = string
  default     = getenv("ATLAS_REVISIONS_SCHEMA")
  description = "Schema holding Atlas's own revisions table."
}

variable "dev_url" {
  type        = string
  default     = getenv("ATLAS_DEV_URL")
  description = "Scratch database Atlas rebuilds on each run. Never real data."
}

locals {
  # Atlas's default is a schema of its own. Some managed providers (Neon among
  # them) trip over creating it, so the schema is configurable and those
  # targets set ATLAS_REVISIONS_SCHEMA=public instead.
  revisions_schema = var.revisions_schema != "" ? var.revisions_schema : "atlas_schema_revisions"

  # Falls back to a conventional local scratch database so the common case
  # needs no extra environment variable.
  dev_url = var.dev_url != "" ? var.dev_url : "postgres://localhost:5432/bscale_atlas_dev?sslmode=disable"
}

env "local" {
  url = var.database_url
  dev = local.dev_url

  migration {
    dir              = "file://db/migrations"
    revisions_schema = local.revisions_schema
  }
}

# Used by CI or any run that passes the target explicitly with --url.
env "ci" {
  dev = local.dev_url

  migration {
    dir              = "file://db/migrations"
    revisions_schema = local.revisions_schema
  }
}
