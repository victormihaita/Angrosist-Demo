# production environment root — thin wrapper over the platform module.
# Separate GCP project + separate state (CLAUDE.md §6). HA + protection on.

module "platform" {
  source = "../../modules/platform"

  project_id  = var.project_id
  region      = var.region
  environment = "prod"

  image_tag            = var.image_tag
  cors_allowed_origins = var.cors_allowed_origins
  db_password          = var.db_password

  # prod: protected + HA.
  deletion_protection  = true
  gcs_force_destroy    = false
  db_tier              = "db-custom-2-7680"
  db_availability_type = "REGIONAL" # HA
  db_retained_backups  = 30

  api_min_instances    = 2
  api_max_instances    = 10
  api_cpu              = "2"
  api_memory           = "1Gi"
  worker_min_instances = 1
  worker_max_instances = 10
  worker_cpu           = "2"
  worker_memory        = "1Gi"

  # worker_container_command = ["/app/worker"]  # enable once image builds it (M2)

  scheduler_jobs = {
    conversation-timeouts = {
      description = "Sweep stale conversations and apply timeouts."
      schedule    = "*/10 * * * *"
      path        = "/api/jobs/conversation-timeouts"
    }
    reminders = {
      description = "Send re-engagement reminders."
      schedule    = "0 9 * * *"
      path        = "/api/jobs/reminders"
    }
    retention-purge = {
      description = "Purge data past its retention window (SECURITY.md §7.3)."
      schedule    = "0 3 * * *"
      path        = "/api/jobs/retention-purge"
    }
  }
}
