# platform — composition module that wires all GCP resources for ONE
# environment (ARCHITECTURE.md §6). The env roots (envs/dev|staging|prod) are
# thin: they set the backend, provider, and pass per-env variables to this
# module. This keeps modules reusable and avoids per-env duplication.
#
# DEFER-PROVISIONING NOTE: nothing here auto-runs. `terraform validate` works
# offline (-backend=false); `plan`/`apply` require real GCP credentials that do
# not exist yet. See deploy/terraform/README.md.

locals {
  name_prefix = "${var.app_short_name}-${var.environment}"

  common_labels = merge(
    {
      app         = var.app_short_name
      environment = var.environment
      managed_by  = "terraform"
    },
    var.labels,
  )

  # Image reference for both Cloud Run services. Both default to the api server
  # entrypoint; the worker switches to /app/worker once the image builds it (M2).
  image = "${module.artifact_registry.registry_base}/${var.image_name}:${var.image_tag}"
}

# --- Identities (least privilege) -------------------------------------------
module "iam" {
  source = "../iam_service_accounts"

  project_id  = var.project_id
  environment = var.environment
  name_prefix = local.name_prefix
}

# Dedicated SA that Cloud Tasks and Cloud Scheduler use to mint OIDC tokens to
# invoke the worker/api endpoints. Held separately from runtime SAs.
resource "google_service_account" "invoker" {
  project      = var.project_id
  account_id   = "${local.name_prefix}-invoker"
  display_name = "Cloud Tasks/Scheduler invoker (${var.environment})"
  description  = "Mints OIDC tokens to invoke Cloud Run worker/job endpoints."
}

# --- Image registry ----------------------------------------------------------
module "artifact_registry" {
  source = "../artifact_registry"

  project_id    = var.project_id
  region        = var.region
  environment   = var.environment
  repository_id = var.image_name
  labels        = local.common_labels
}

# --- Secrets (names only; values added out-of-band) -------------------------
module "secrets" {
  source = "../secret_manager"

  project_id             = var.project_id
  name_prefix            = local.name_prefix
  app_accessor_sa_emails = [module.iam.api_sa_email, module.iam.worker_sa_email]
  replica_regions        = var.secret_replica_regions
  labels                 = local.common_labels
}

# --- Database ----------------------------------------------------------------
module "cloud_sql" {
  source = "../cloud_sql"

  project_id          = var.project_id
  region              = var.region
  instance_name       = "${local.name_prefix}-pg"
  tier                = var.db_tier
  availability_type   = var.db_availability_type
  disk_size_gb        = var.db_disk_size_gb
  deletion_protection = var.deletion_protection
  retained_backups    = var.db_retained_backups
  public_ip_enabled   = var.db_public_ip_enabled
  private_network_id  = var.private_network_id
  database_name       = var.db_name
  database_user       = var.db_user
  database_password   = var.db_password # from Secret Manager at apply time
  labels              = local.common_labels
}

# --- Documents bucket --------------------------------------------------------
module "gcs" {
  source = "../gcs"

  project_id             = var.project_id
  bucket_name            = "${local.name_prefix}-documents-${var.project_id}"
  location               = var.gcs_location
  force_destroy          = var.gcs_force_destroy
  object_admin_sa_emails = [module.iam.api_sa_email, module.iam.worker_sa_email]
  labels                 = local.common_labels
}

# --- Async queue -------------------------------------------------------------
module "cloud_tasks" {
  source = "../cloud_tasks"

  project_id = var.project_id
  region     = var.region
  queue_name = "${local.name_prefix}-agent-turns"
}

# --- Cloud Run: api ----------------------------------------------------------
module "run_api" {
  source = "../cloud_run_service"

  project_id            = var.project_id
  region                = var.region
  service_name          = "${local.name_prefix}-api"
  image                 = local.image
  service_account_email = module.iam.api_sa_email
  container_command     = var.api_container_command

  min_instances = var.api_min_instances
  max_instances = var.api_max_instances
  cpu           = var.api_cpu
  memory        = var.api_memory

  ingress               = "INGRESS_TRAFFIC_ALL"
  allow_unauthenticated = true # public; protected by Cloudflare WAF + app auth

  cloudsql_connection_name = module.cloud_sql.connection_name
  deletion_protection      = var.deletion_protection

  env = merge(
    {
      ENVIRONMENT          = var.environment
      GCP_PROJECT_ID       = var.project_id
      GCP_REGION           = var.region
      GCS_BUCKET           = module.gcs.bucket_name
      CLOUD_TASKS_QUEUE    = module.cloud_tasks.queue_name
      CLOUDSQL_CONNECTION  = module.cloud_sql.connection_name
      DB_NAME              = var.db_name
      DB_USER              = var.db_user
      CORS_ALLOWED_ORIGINS = var.cors_allowed_origins
    },
    var.api_extra_env,
  )

  # Secret env injected by reference (latest version). No values in Terraform.
  secret_env = {
    DB_PASSWORD           = { secret = module.secrets.secret_ids["db-password"], version = "latest" }
    LLM_API_KEY           = { secret = module.secrets.secret_ids["llm-api-key"], version = "latest" }
    WHATSAPP_TOKEN        = { secret = module.secrets.secret_ids["whatsapp-token"], version = "latest" }
    WHATSAPP_APP_SECRET   = { secret = module.secrets.secret_ids["whatsapp-app-secret"], version = "latest" }
    WHATSAPP_VERIFY_TOKEN = { secret = module.secrets.secret_ids["whatsapp-verify-token"], version = "latest" }
    EMAIL_API_KEY         = { secret = module.secrets.secret_ids["email-api-key"], version = "latest" }
    JWT_SECRET            = { secret = module.secrets.secret_ids["jwt-secret"], version = "latest" }
  }

  labels = local.common_labels
}

# --- Cloud Run: worker (Cloud Tasks push target) ----------------------------
module "run_worker" {
  source = "../cloud_run_service"

  project_id            = var.project_id
  region                = var.region
  service_name          = "${local.name_prefix}-worker"
  image                 = local.image
  service_account_email = module.iam.worker_sa_email
  container_command     = var.worker_container_command

  min_instances = var.worker_min_instances
  max_instances = var.worker_max_instances
  cpu           = var.worker_cpu
  memory        = var.worker_memory

  # Worker is NOT public. Only the invoker SA (Cloud Tasks/Scheduler) may call.
  ingress               = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  allow_unauthenticated = false
  invoker_sa_emails     = [google_service_account.invoker.email]

  cloudsql_connection_name = module.cloud_sql.connection_name
  deletion_protection      = var.deletion_protection

  env = merge(
    {
      ENVIRONMENT         = var.environment
      GCP_PROJECT_ID      = var.project_id
      GCP_REGION          = var.region
      GCS_BUCKET          = module.gcs.bucket_name
      CLOUD_TASKS_QUEUE   = module.cloud_tasks.queue_name
      CLOUDSQL_CONNECTION = module.cloud_sql.connection_name
      DB_NAME             = var.db_name
      DB_USER             = var.db_user
    },
    var.worker_extra_env,
  )

  secret_env = {
    DB_PASSWORD         = { secret = module.secrets.secret_ids["db-password"], version = "latest" }
    LLM_API_KEY         = { secret = module.secrets.secret_ids["llm-api-key"], version = "latest" }
    WHATSAPP_TOKEN      = { secret = module.secrets.secret_ids["whatsapp-token"], version = "latest" }
    WHATSAPP_APP_SECRET = { secret = module.secrets.secret_ids["whatsapp-app-secret"], version = "latest" }
    EMAIL_API_KEY       = { secret = module.secrets.secret_ids["email-api-key"], version = "latest" }
    JWT_SECRET          = { secret = module.secrets.secret_ids["jwt-secret"], version = "latest" }
  }

  labels = local.common_labels
}

# --- Cron --------------------------------------------------------------------
module "cloud_scheduler" {
  source = "../cloud_scheduler"

  project_id         = var.project_id
  region             = var.region
  name_prefix        = local.name_prefix
  time_zone          = var.scheduler_time_zone
  scheduler_sa_email = google_service_account.invoker.email
  target_base_url    = module.run_worker.uri
  jobs               = var.scheduler_jobs
}
