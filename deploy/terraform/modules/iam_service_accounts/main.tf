# Service accounts — least-privilege identities (SECURITY.md §9.3).
#
# Three dedicated service accounts per environment:
#   - api:      runtime identity for the Cloud Run "api" service
#   - worker:   runtime identity for the Cloud Run "worker" service
#   - deployer: identity CI impersonates (via Workload Identity Federation) to
#               build/push images, deploy Cloud Run, and run migrations.
#
# Project-level role grants here are intentionally minimal. Resource-scoped
# grants (Secret Manager accessor per secret, GCS object access on the bucket,
# Cloud Tasks enqueuer on the queue, Cloud Run invoker on the worker) live in
# the modules that own those resources so each grant is least-privilege and
# scoped by name. No default/over-broad service accounts are used.

resource "google_service_account" "api" {
  project      = var.project_id
  account_id   = "${var.name_prefix}-api"
  display_name = "Cloud Run API service (${var.environment})"
  description  = "Runtime identity for the api Cloud Run service."
}

resource "google_service_account" "worker" {
  project      = var.project_id
  account_id   = "${var.name_prefix}-worker"
  display_name = "Cloud Run worker service (${var.environment})"
  description  = "Runtime identity for the worker Cloud Run service (Cloud Tasks push target)."
}

resource "google_service_account" "deployer" {
  project      = var.project_id
  account_id   = "${var.name_prefix}-deployer"
  display_name = "CI deployer (${var.environment})"
  description  = "Identity CI impersonates via Workload Identity Federation to build/deploy. No exported keys."
}

# --- Cloud SQL client: both runtime SAs connect to the database. ------------
resource "google_project_iam_member" "api_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.api.email}"
}

resource "google_project_iam_member" "worker_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.worker.email}"
}

# --- Logging: runtime SAs write structured logs. ----------------------------
resource "google_project_iam_member" "api_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.api.email}"
}

resource "google_project_iam_member" "worker_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.worker.email}"
}

# --- Deployer (CI) roles. ---------------------------------------------------
# Push images to Artifact Registry.
resource "google_project_iam_member" "deployer_ar_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.deployer.email}"
}

# Deploy/update Cloud Run revisions.
resource "google_project_iam_member" "deployer_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.deployer.email}"
}

# Run Cloud SQL Auth Proxy / connect to run migrations during deploy.
resource "google_project_iam_member" "deployer_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.deployer.email}"
}

# CI must be able to act as the runtime SAs to deploy services that run as them
# (Cloud Run requires actAs on the service's runtime SA).
resource "google_service_account_iam_member" "deployer_act_as_api" {
  service_account_id = google_service_account.api.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.deployer.email}"
}

resource "google_service_account_iam_member" "deployer_act_as_worker" {
  service_account_id = google_service_account.worker.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.deployer.email}"
}
