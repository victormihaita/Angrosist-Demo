# Artifact Registry — Docker repository for the backend image.
#
# One Docker repo per environment/project. Images are pushed by CI (Workload
# Identity, never a laptop) and pulled by the Cloud Run service accounts.
# EU region only (data residency).

resource "google_artifact_registry_repository" "docker" {
  project       = var.project_id
  location      = var.region
  repository_id = var.repository_id
  description   = "Docker images for the Euro Intermed backend (${var.environment})"
  format        = "DOCKER"

  # Keep image scanning on so vulnerabilities are caught before deploy
  # (SECURITY.md §10). Cleanup policies keep storage costs bounded.
  docker_config {
    immutable_tags = false
  }

  # Retain only the most recent N untagged images; delete older ones.
  cleanup_policies {
    id     = "keep-recent-untagged"
    action = "KEEP"
    most_recent_versions {
      keep_count = var.keep_recent_versions
    }
  }

  labels = var.labels
}
