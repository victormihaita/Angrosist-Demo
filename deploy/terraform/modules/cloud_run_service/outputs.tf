output "service_name" {
  description = "Cloud Run service name."
  value       = google_cloud_run_v2_service.this.name
}

output "uri" {
  description = "Public URI of the service (managed *.run.app URL)."
  value       = google_cloud_run_v2_service.this.uri
}
