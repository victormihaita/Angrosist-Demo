output "registry_base" {
  description = "Artifact Registry base path for image tags."
  value       = module.artifact_registry.registry_base
}

output "api_url" {
  description = "Cloud Run api service URL."
  value       = module.run_api.uri
}

output "worker_url" {
  description = "Cloud Run worker service URL."
  value       = module.run_worker.uri
}

output "cloudsql_connection_name" {
  description = "Cloud SQL instance connection name."
  value       = module.cloud_sql.connection_name
}

output "documents_bucket" {
  description = "GCS documents bucket name."
  value       = module.gcs.bucket_name
}

output "cloud_tasks_queue" {
  description = "Cloud Tasks queue name."
  value       = module.cloud_tasks.queue_name
}

output "api_sa_email" {
  description = "api runtime service account."
  value       = module.iam.api_sa_email
}

output "worker_sa_email" {
  description = "worker runtime service account."
  value       = module.iam.worker_sa_email
}

output "deployer_sa_email" {
  description = "CI deployer service account (impersonated via Workload Identity)."
  value       = module.iam.deployer_sa_email
}

output "invoker_sa_email" {
  description = "Cloud Tasks/Scheduler invoker service account."
  value       = google_service_account.invoker.email
}

output "secret_ids" {
  description = "Logical secret key -> Secret Manager secret_id (no values)."
  value       = module.secrets.secret_ids
}
