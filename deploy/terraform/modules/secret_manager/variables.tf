variable "project_id" {
  description = "GCP project ID for this environment."
  type        = string
}

variable "name_prefix" {
  description = "Prefix for secret IDs (e.g. ei-dev), to disambiguate per env."
  type        = string
}

variable "app_accessor_sa_emails" {
  description = "Service account emails (api, worker) granted accessor on every app secret."
  type        = list(string)
}

variable "replica_regions" {
  description = "EU regions for user-managed secret replication (data residency)."
  type        = list(string)
  default     = ["europe-west1", "europe-west4"]
}

variable "labels" {
  description = "Resource labels."
  type        = map(string)
  default     = {}
}
