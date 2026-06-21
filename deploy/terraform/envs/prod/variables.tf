variable "project_id" {
  description = "GCP project ID for the production environment."
  type        = string
}

variable "region" {
  description = "Primary GCP region (europe-*)."
  type        = string
  default     = "europe-west1"
}

variable "image_tag" {
  description = "Backend image tag to deploy (CI sets to commit SHA)."
  type        = string
  default     = "latest"
}

variable "cors_allowed_origins" {
  description = "Comma-separated CORS allow-list (NO '*' in prod)."
  type        = string
  default     = ""
}

variable "db_password" {
  description = "App DB password from Secret Manager at apply time. NEVER commit."
  type        = string
  sensitive   = true
}
