variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "region" {
  description = "GCP region (europe-*)."
  type        = string
}

variable "name_prefix" {
  description = "Prefix for scheduler job names."
  type        = string
}

variable "time_zone" {
  description = "Cron time zone."
  type        = string
  default     = "Europe/Bucharest"
}

variable "scheduler_sa_email" {
  description = "Service account email used to mint OIDC tokens for the target."
  type        = string
}

variable "target_base_url" {
  description = "Base URL of the Cloud Run service the jobs POST to."
  type        = string
}

variable "jobs" {
  description = "Map of cron jobs (placeholders for timeouts/reminders/matching)."
  type = map(object({
    description      = string
    schedule         = string # cron expression
    path             = string # endpoint path appended to target_base_url
    attempt_deadline = optional(string, "320s")
    retry_count      = optional(number, 3)
  }))
  default = {}
}
