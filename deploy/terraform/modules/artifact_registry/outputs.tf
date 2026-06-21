output "repository_id" {
  description = "The Artifact Registry repository ID."
  value       = google_artifact_registry_repository.docker.repository_id
}

output "repository_name" {
  description = "Fully qualified repository name."
  value       = google_artifact_registry_repository.docker.name
}

# Base path images are pushed/pulled from, e.g.
# europe-west1-docker.pkg.dev/PROJECT/REPO
output "registry_base" {
  description = "Docker registry base path for image tags."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.docker.repository_id}"
}
