variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "region" {
  description = "GCP region (europe-*)."
  type        = string
}

variable "queue_name" {
  description = "Cloud Tasks queue name."
  type        = string
}

variable "max_dispatches_per_second" {
  description = "Max task dispatches per second."
  type        = number
  default     = 50
}

variable "max_concurrent_dispatches" {
  description = "Max concurrent task dispatches."
  type        = number
  default     = 20
}

variable "max_attempts" {
  description = "Max delivery attempts before a task is dropped."
  type        = number
  default     = 8
}

variable "min_backoff" {
  description = "Minimum retry backoff."
  type        = string
  default     = "5s"
}

variable "max_backoff" {
  description = "Maximum retry backoff."
  type        = string
  default     = "300s"
}

variable "max_doublings" {
  description = "Number of times backoff doubles."
  type        = number
  default     = 5
}

variable "max_retry_duration" {
  description = "Total time a task may be retried."
  type        = string
  default     = "3600s"
}

variable "log_sampling_ratio" {
  description = "Stackdriver logging sampling ratio (0-1)."
  type        = number
  default     = 1.0
}
