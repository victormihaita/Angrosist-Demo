output "api_sa_email" {
  description = "Email of the api runtime service account."
  value       = google_service_account.api.email
}

output "worker_sa_email" {
  description = "Email of the worker runtime service account."
  value       = google_service_account.worker.email
}

output "deployer_sa_email" {
  description = "Email of the CI deployer service account."
  value       = google_service_account.deployer.email
}
