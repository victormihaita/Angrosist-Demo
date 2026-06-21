variable "project_id" {
  description = "GCP project ID for this environment."
  type        = string
}

variable "bucket_name" {
  description = "Globally-unique bucket name."
  type        = string
}

variable "location" {
  description = "Bucket location (EU multi-region or a europe-* region)."
  type        = string
  default     = "EU"
}

variable "force_destroy" {
  description = "Allow Terraform to delete a non-empty bucket. Keep false for prod."
  type        = bool
  default     = false
}

variable "keep_noncurrent_versions" {
  description = "Max noncurrent object versions retained before deletion."
  type        = number
  default     = 3
}

variable "noncurrent_retention_days" {
  description = "Delete noncurrent versions older than this many days."
  type        = number
  default     = 90
}

variable "object_admin_sa_emails" {
  description = "Service account emails granted scoped object access (api, worker)."
  type        = list(string)
  default     = []
}

variable "labels" {
  description = "Resource labels."
  type        = map(string)
  default     = {}
}
