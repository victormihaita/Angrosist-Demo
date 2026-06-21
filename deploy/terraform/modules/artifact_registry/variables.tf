variable "project_id" {
  description = "GCP project ID for this environment."
  type        = string
}

variable "region" {
  description = "GCP region (must be europe-* for EU data residency)."
  type        = string
}

variable "environment" {
  description = "Environment name (dev/staging/prod)."
  type        = string
}

variable "repository_id" {
  description = "Artifact Registry Docker repository ID."
  type        = string
}

variable "keep_recent_versions" {
  description = "Number of recent untagged image versions to retain."
  type        = number
  default     = 10
}

variable "labels" {
  description = "Resource labels."
  type        = map(string)
  default     = {}
}
