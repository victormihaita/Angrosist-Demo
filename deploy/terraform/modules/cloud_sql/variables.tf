variable "project_id" {
  description = "GCP project ID for this environment."
  type        = string
}

variable "region" {
  description = "GCP region (must be europe-* for EU data residency)."
  type        = string
}

variable "instance_name" {
  description = "Cloud SQL instance name."
  type        = string
}

variable "database_version" {
  description = "Postgres engine version."
  type        = string
  default     = "POSTGRES_16"
}

variable "tier" {
  description = "Machine tier (e.g. db-custom-1-3840 prod, db-f1-micro dev)."
  type        = string
}

variable "availability_type" {
  description = "ZONAL (dev/staging) or REGIONAL (prod HA)."
  type        = string
  default     = "ZONAL"
}

variable "disk_size_gb" {
  description = "Initial disk size in GB (autoresize enabled)."
  type        = number
  default     = 10
}

variable "deletion_protection" {
  description = "Block accidental instance deletion."
  type        = bool
  default     = true
}

variable "backup_start_time" {
  description = "Daily backup start time (HH:MM UTC)."
  type        = string
  default     = "02:00"
}

variable "transaction_log_retention_days" {
  description = "Days of transaction logs retained for PITR (1-7)."
  type        = number
  default     = 7
}

variable "retained_backups" {
  description = "Number of automated backups retained."
  type        = number
  default     = 14
}

variable "backup_location" {
  description = "Backup storage location (EU)."
  type        = string
  default     = "eu"
}

variable "public_ip_enabled" {
  description = "Enable a public IPv4 address. Keep false; use private IP."
  type        = bool
  default     = false
}

variable "private_network_id" {
  description = "Self-link of the VPC network for private IP. Null disables private IP."
  type        = string
  default     = null
}

variable "database_name" {
  description = "Application database name."
  type        = string
  default     = "angrosist"
}

variable "database_user" {
  description = "Application database user."
  type        = string
  default     = "angrosist"
}

variable "database_password" {
  description = "Application DB user password. Supplied at apply time from Secret Manager; never committed."
  type        = string
  sensitive   = true
}

variable "labels" {
  description = "Resource labels."
  type        = map(string)
  default     = {}
}
