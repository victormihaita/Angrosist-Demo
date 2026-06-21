# Cloud Tasks queue — async agent turns (ARCHITECTURE.md §4, §6).
#
# The queue's tasks are HTTP-push targets to the worker Cloud Run service. The
# push request is authenticated with an OIDC token minted for the queue's
# invoker service account, which holds run.invoker on the worker (granted in the
# env root via the cloud_run_service.invoker_sa_emails). EU region only.
#
# Retry config gives durable, backed-off retries (worker classifies retryable
# vs terminal per backend/CLAUDE.md). The actual task creation (with the worker
# URL + OIDC) happens at runtime in the queue adapter; this resource defines the
# queue and its retry/rate policy.

resource "google_cloud_tasks_queue" "agent_turns" {
  project  = var.project_id
  location = var.region
  name     = var.queue_name

  rate_limits {
    max_dispatches_per_second = var.max_dispatches_per_second
    max_concurrent_dispatches = var.max_concurrent_dispatches
  }

  retry_config {
    max_attempts       = var.max_attempts
    min_backoff        = var.min_backoff
    max_backoff        = var.max_backoff
    max_doublings      = var.max_doublings
    max_retry_duration = var.max_retry_duration
  }

  stackdriver_logging_config {
    sampling_ratio = var.log_sampling_ratio
  }
}
