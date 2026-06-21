# Cloud SQL for PostgreSQL (ARCHITECTURE.md §6, SECURITY.md §7.5).
#
# - EU region only (data residency).
# - Automated daily backups + Point-In-Time Recovery (binary logging / WAL).
# - Private IP via the project's VPC where a network is supplied; public IP is
#   disabled by default (SECURITY.md §9). Cloud Run connects via the Cloud SQL
#   connector using the instance connection name + IAM.
# - The DB password is NOT defined here. The application user's password is set
#   out-of-band and stored in Secret Manager (db-password); Terraform creates
#   the user with a value supplied at apply time via a sensitive variable that
#   the operator passes from the secret, never committed.

resource "google_sql_database_instance" "this" {
  project          = var.project_id
  name             = var.instance_name
  region           = var.region
  database_version = var.database_version

  # Guard against accidental destruction of the primary database.
  deletion_protection = var.deletion_protection

  settings {
    tier              = var.tier
    availability_type = var.availability_type # ZONAL (dev) / REGIONAL (prod HA)
    disk_type         = "PD_SSD"
    disk_size         = var.disk_size_gb
    disk_autoresize   = true

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true # PITR (required)
      start_time                     = var.backup_start_time
      transaction_log_retention_days = var.transaction_log_retention_days
      location                       = var.backup_location # EU
      backup_retention_settings {
        retained_backups = var.retained_backups
        retention_unit   = "COUNT"
      }
    }

    ip_configuration {
      # Private IP only by default. Public IP stays off for data residency and
      # to keep the DB off the public internet (SECURITY.md §9).
      ipv4_enabled    = var.public_ip_enabled
      private_network = var.private_network_id
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    database_flags {
      name  = "cloudsql.iam_authentication"
      value = "on"
    }

    insights_config {
      query_insights_enabled  = true
      record_application_tags = false
      record_client_address   = false # no PII (client IPs) in query insights
    }

    user_labels = var.labels
  }
}

resource "google_sql_database" "app" {
  project  = var.project_id
  instance = google_sql_database_instance.this.name
  name     = var.database_name
}

# Application DB user. The password comes from a sensitive variable that the
# operator wires from Secret Manager at apply time — never hardcoded, never in
# state files committed to git (state lives in the locked-down GCS backend).
resource "google_sql_user" "app" {
  project  = var.project_id
  instance = google_sql_database_instance.this.name
  name     = var.database_user
  password = var.database_password
}
