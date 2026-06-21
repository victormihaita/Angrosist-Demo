# Reusable Cloud Run (v2) service — used for both `api` and `worker`
# (ARCHITECTURE.md §6). Same Docker image, different container command and
# different ingress/invoker policy.
#
# - EU region only.
# - min-instances configurable (>= 1 for the warm api so WebSockets stay up).
# - Secrets injected as env from Secret Manager BY REFERENCE (latest version);
#   no secret values pass through Terraform.
# - Cloud SQL attached via the connector (instance connection name).
# - Runs as a dedicated least-privilege service account.
#
# NOTE on the worker command: the backend Dockerfile currently builds `server`
# and `migrate` only; the `worker` binary lands in M2. Until then, set
# `container_command` to ["/app/server"] for both services and switch the worker
# to ["/app/worker"] once the image builds it. The command is a variable, so no
# image or module change is needed then.

resource "google_cloud_run_v2_service" "this" {
  project  = var.project_id
  location = var.region
  name     = var.service_name

  # Internal-only ingress for the worker (reached by Cloud Tasks over the
  # internal path / OIDC); the api takes external traffic (fronted by Cloudflare).
  ingress = var.ingress

  deletion_protection = var.deletion_protection

  template {
    service_account = var.service_account_email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    # Attach Cloud SQL via the built-in connector when an instance is supplied.
    dynamic "volumes" {
      for_each = var.cloudsql_connection_name == null ? [] : [1]
      content {
        name = "cloudsql"
        cloud_sql_instance {
          instances = [var.cloudsql_connection_name]
        }
      }
    }

    containers {
      image = var.image

      # Override the image's default CMD per service (api vs worker).
      command = var.container_command
      args    = var.container_args

      ports {
        container_port = var.container_port
      }

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
        cpu_idle          = var.min_instances == 0
        startup_cpu_boost = true
      }

      # Plain (non-secret) config env.
      dynamic "env" {
        for_each = var.env
        content {
          name  = env.key
          value = env.value
        }
      }

      # Secret env — referenced by Secret Manager secret name + version.
      # secret_env maps ENV_VAR_NAME -> { secret = "<secret_id>", version = "latest" }.
      dynamic "env" {
        for_each = var.secret_env
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value.secret
              version = env.value.version
            }
          }
        }
      }

      dynamic "volume_mounts" {
        for_each = var.cloudsql_connection_name == null ? [] : [1]
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }

      startup_probe {
        http_get {
          path = var.health_path
        }
        initial_delay_seconds = 5
        timeout_seconds       = 3
        period_seconds        = 10
        failure_threshold     = 6
      }
    }

    timeout                          = var.request_timeout
    max_instance_request_concurrency = var.concurrency
  }

  labels = var.labels
}

# Invoker policy.
# - api: public (allUsers) — protected by Cloudflare WAF + app-level auth.
# - worker: NO public invoker; only the specified caller SAs (Cloud Tasks /
#   Cloud Scheduler OIDC identity) may invoke.
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  count = var.allow_unauthenticated ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_v2_service_iam_member" "invoker_sa" {
  for_each = toset(var.invoker_sa_emails)

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${each.value}"
}
