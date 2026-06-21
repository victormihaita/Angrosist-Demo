output "bucket_name" {
  description = "Name of the documents bucket."
  value       = google_storage_bucket.documents.name
}

output "bucket_url" {
  description = "gs:// URL of the documents bucket."
  value       = google_storage_bucket.documents.url
}
