# staging environment root — thin wrapper over the platform module.
# Separate GCP project + separate state (CLAUDE.md §6).

module "platform" {
  source = "../../modules/platform"

  project_id  = var.project_id
  region      = var.region
  environment = "staging"

  image_tag            = var.image_tag
  cors_allowed_origins = var.cors_allowed_origins
  db_password          = var.db_password

  # staging mirrors prod shape but smaller.
  deletion_protection  = true
  gcs_force_destroy    = false
  db_tier              = "db-custom-1-3840"
  db_availability_type = "ZONAL"
  db_retained_backups  = 14

  api_min_instances    = 1
  api_max_instances    = 4
  worker_min_instances = 0
  worker_max_instances = 4

  # worker_container_command = ["/app/worker"]  # enable once image builds it (M2)

  scheduler_jobs = {
    conversation-timeouts = {
      description = "Sweep stale conversations and apply timeouts."
      schedule    = "*/15 * * * *"
      path        = "/api/jobs/conversation-timeouts"
    }
    reminders = {
      description = "Send re-engagement reminders."
      schedule    = "0 9 * * *"
      path        = "/api/jobs/reminders"
    }
  }
}
