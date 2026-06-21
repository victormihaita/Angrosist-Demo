# Cloud Scheduler — cron job endpoints (ARCHITECTURE.md §6).
#
# Placeholders for Phase-1 scheduled work: conversation timeouts, re-engagement
# reminders, and (P2) matching timeouts. Each job POSTs to an HTTP endpoint on
# the api/worker service, authenticated with an OIDC token for the scheduler
# service account (which must hold run.invoker on the target service). EU region.
#
# Jobs are defined from var.jobs so environments add/remove cron without code
# duplication. The target base URL is supplied by the env root once the service
# exists; until provisioning, an empty jobs map creates nothing.

resource "google_cloud_scheduler_job" "jobs" {
  for_each = var.jobs

  project     = var.project_id
  region      = var.region
  name        = "${var.name_prefix}-${each.key}"
  description = each.value.description
  schedule    = each.value.schedule
  time_zone   = var.time_zone

  attempt_deadline = each.value.attempt_deadline

  retry_config {
    retry_count = each.value.retry_count
  }

  http_target {
    http_method = "POST"
    uri         = "${var.target_base_url}${each.value.path}"

    oidc_token {
      service_account_email = var.scheduler_sa_email
      audience              = var.target_base_url
    }

    headers = {
      "Content-Type" = "application/json"
    }
  }
}
