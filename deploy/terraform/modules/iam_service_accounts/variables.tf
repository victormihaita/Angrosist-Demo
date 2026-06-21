variable "project_id" {
  description = "GCP project ID for this environment."
  type        = string
}

variable "environment" {
  description = "Environment name (dev/staging/prod)."
  type        = string
}

variable "name_prefix" {
  description = "Prefix for service account IDs (e.g. ei-dev)."
  type        = string
}
