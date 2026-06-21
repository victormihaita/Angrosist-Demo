# --- Core identity / region --------------------------------------------------
variable "project_id" {
  description = "GCP project ID for this environment (separate project per env)."
  type        = string
}

variable "region" {
  description = "Primary GCP region. MUST be europe-* (EU data residency)."
  type        = string
  default     = "europe-west1"

  validation {
    condition     = startswith(var.region, "europe-")
    error_message = "EU data residency: region must be in europe-*."
  }
}

variable "environment" {
  description = "Environment name."
  type        = string

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "environment must be one of dev, staging, prod."
  }
}

variable "app_short_name" {
  description = "Short app slug used in resource names (lowercase, no spaces)."
  type        = string
  default     = "ei"
}

variable "labels" {
  description = "Extra labels merged into every resource."
  type        = map(string)
  default     = {}
}

variable "deletion_protection" {
  description = "Enable deletion protection on stateful resources (DB, services)."
  type        = bool
  default     = true
}

# --- Image -------------------------------------------------------------------
variable "image_name" {
  description = "Artifact Registry repo + image name (also the AR repository_id)."
  type        = string
  default     = "backend"
}

variable "image_tag" {
  description = "Image tag to deploy (CI sets this to the commit SHA)."
  type        = string
  default     = "latest"
}

variable "api_container_command" {
  description = "Entrypoint for the api service."
  type        = list(string)
  default     = ["/app/server"]
}

variable "worker_container_command" {
  description = "Entrypoint for the worker service. Switch to [\"/app/worker\"] once the image builds it (M2)."
  type        = list(string)
  default     = ["/app/server"]
}

# --- Cloud Run sizing --------------------------------------------------------
variable "api_min_instances" {
  description = "Min instances for api (>=1 keeps it warm for WebSocket)."
  type        = number
  default     = 1
}

variable "api_max_instances" {
  description = "Max instances for api."
  type        = number
  default     = 4
}

variable "api_cpu" {
  description = "api CPU limit."
  type        = string
  default     = "1"
}

variable "api_memory" {
  description = "api memory limit."
  type        = string
  default     = "512Mi"
}

variable "api_extra_env" {
  description = "Additional plain env for api."
  type        = map(string)
  default     = {}
}

variable "worker_min_instances" {
  description = "Min instances for worker."
  type        = number
  default     = 0
}

variable "worker_max_instances" {
  description = "Max instances for worker."
  type        = number
  default     = 4
}

variable "worker_cpu" {
  description = "worker CPU limit."
  type        = string
  default     = "1"
}

variable "worker_memory" {
  description = "worker memory limit."
  type        = string
  default     = "512Mi"
}

variable "worker_extra_env" {
  description = "Additional plain env for worker."
  type        = map(string)
  default     = {}
}

variable "cors_allowed_origins" {
  description = "Comma-separated CORS allow-list (no '*' in prod, SECURITY.md §9.4)."
  type        = string
  default     = ""
}

# --- Database ----------------------------------------------------------------
variable "db_tier" {
  description = "Cloud SQL machine tier."
  type        = string
  default     = "db-f1-micro"
}

variable "db_availability_type" {
  description = "ZONAL or REGIONAL (HA for prod)."
  type        = string
  default     = "ZONAL"
}

variable "db_disk_size_gb" {
  description = "Cloud SQL disk size (autoresize enabled)."
  type        = number
  default     = 10
}

variable "db_retained_backups" {
  description = "Number of automated backups retained."
  type        = number
  default     = 14
}

variable "db_public_ip_enabled" {
  description = "Enable a public IP on Cloud SQL (keep false; use private IP)."
  type        = bool
  default     = false
}

variable "private_network_id" {
  description = "VPC self-link for Cloud SQL private IP (null = no private IP yet)."
  type        = string
  default     = null
}

variable "db_name" {
  description = "Application database name."
  type        = string
  default     = "angrosist"
}

variable "db_user" {
  description = "Application database user."
  type        = string
  default     = "angrosist"
}

variable "db_password" {
  description = "App DB password, passed from Secret Manager at apply time. NEVER commit a value."
  type        = string
  sensitive   = true
}

# --- Storage / secrets locations --------------------------------------------
variable "gcs_location" {
  description = "Documents bucket location (EU)."
  type        = string
  default     = "EU"
}

variable "gcs_force_destroy" {
  description = "Allow deleting a non-empty bucket (keep false for prod)."
  type        = bool
  default     = false
}

variable "secret_replica_regions" {
  description = "EU regions for secret replication."
  type        = list(string)
  default     = ["europe-west1", "europe-west4"]
}

# --- Scheduler ---------------------------------------------------------------
variable "scheduler_time_zone" {
  description = "Cron time zone."
  type        = string
  default     = "Europe/Bucharest"
}

variable "scheduler_jobs" {
  description = "Cron job placeholders (timeouts/reminders/[P2] matching)."
  type = map(object({
    description      = string
    schedule         = string
    path             = string
    attempt_deadline = optional(string, "320s")
    retry_count      = optional(number, 3)
  }))
  default = {}
}
