# dev environment root — thin wrapper over the platform module.
# Separate GCP project + separate state (CLAUDE.md §6).

module "platform" {
  source = "../../modules/platform"

  project_id  = var.project_id
  region      = var.region
  environment = "dev"

  image_tag            = var.image_tag
  cors_allowed_origins = var.cors_allowed_origins
  db_password          = var.db_password

  # dev: cheap + disposable.
  deletion_protection  = false
  gcs_force_destroy    = true
  db_tier              = "db-f1-micro"
  db_availability_type = "ZONAL"
  db_retained_backups  = 7

  api_min_instances    = 0
  api_max_instances    = 2
  worker_min_instances = 0
  worker_max_instances = 2

  # Worker entrypoint stays /app/server until the image builds /app/worker (M2).
  # worker_container_command = ["/app/worker"]

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
