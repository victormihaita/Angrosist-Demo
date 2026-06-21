variable "project_id" {
  description = "GCP project ID for this environment."
  type        = string
}

variable "region" {
  description = "GCP region (must be europe-*)."
  type        = string
}

variable "service_name" {
  description = "Cloud Run service name."
  type        = string
}

variable "image" {
  description = "Fully-qualified container image (Artifact Registry path + tag/digest)."
  type        = string
}

variable "service_account_email" {
  description = "Runtime service account email for this service."
  type        = string
}

variable "container_command" {
  description = "Container entrypoint override (e.g. [\"/app/server\"] or [\"/app/worker\"])."
  type        = list(string)
  default     = null
}

variable "container_args" {
  description = "Container args."
  type        = list(string)
  default     = null
}

variable "container_port" {
  description = "Port the container listens on."
  type        = number
  default     = 8080
}

variable "health_path" {
  description = "HTTP path for the startup probe."
  type        = string
  default     = "/api/health"
}

variable "min_instances" {
  description = "Minimum instances (>=1 for the warm api / WebSocket support)."
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum instances."
  type        = number
  default     = 4
}

variable "cpu" {
  description = "CPU limit per instance."
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory limit per instance."
  type        = string
  default     = "512Mi"
}

variable "concurrency" {
  description = "Max concurrent requests per instance."
  type        = number
  default     = 80
}

variable "request_timeout" {
  description = "Request timeout (e.g. 300s)."
  type        = string
  default     = "300s"
}

variable "ingress" {
  description = "Ingress setting: INGRESS_TRAFFIC_ALL or INGRESS_TRAFFIC_INTERNAL_ONLY."
  type        = string
  default     = "INGRESS_TRAFFIC_ALL"
}

variable "allow_unauthenticated" {
  description = "Grant allUsers run.invoker (true for api, false for worker)."
  type        = bool
  default     = false
}

variable "invoker_sa_emails" {
  description = "Service account emails granted run.invoker (e.g. Cloud Tasks/Scheduler for the worker)."
  type        = list(string)
  default     = []
}

variable "cloudsql_connection_name" {
  description = "Cloud SQL instance connection name to attach, or null for none."
  type        = string
  default     = null
}

variable "env" {
  description = "Plain (non-secret) environment variables."
  type        = map(string)
  default     = {}
}

variable "secret_env" {
  description = "Secret env: ENV_VAR -> { secret = secret_id, version = \"latest\" }."
  type = map(object({
    secret  = string
    version = string
  }))
  default = {}
}

variable "deletion_protection" {
  description = "Block accidental service deletion."
  type        = bool
  default     = false
}

variable "labels" {
  description = "Resource labels."
  type        = map(string)
  default     = {}
}
