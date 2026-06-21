# GCS documents bucket (ARCHITECTURE.md §6, SECURITY.md §1.6 / §7.5).
#
# - EU location (data residency).
# - Uniform bucket-level access (no per-object ACLs).
# - Public access prevention enforced: files are served via SIGNED URLs only,
#   never public (SECURITY.md §1.6).
# - Object versioning ON so erasure can purge versions (SECURITY.md §7.4) and
#   for accidental-delete recovery (ARCHITECTURE.md §8).
# - Lifecycle: keep a bounded number of noncurrent versions to control cost.
# - Object access is scoped to the runtime service accounts only (objectAdmin),
#   so they can read/write/sign — no broad project grant.

resource "google_storage_bucket" "documents" {
  project                     = var.project_id
  name                        = var.bucket_name
  location                    = var.location # EU
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  force_destroy = var.force_destroy # false for prod

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      num_newer_versions = var.keep_noncurrent_versions
    }
    action {
      type = "Delete"
    }
  }

  lifecycle_rule {
    condition {
      days_since_noncurrent_time = var.noncurrent_retention_days
    }
    action {
      type = "Delete"
    }
  }

  labels = var.labels
}

# Scoped object access for runtime SAs (read/write + signed-URL generation).
resource "google_storage_bucket_iam_member" "object_admin" {
  for_each = toset(var.object_admin_sa_emails)

  bucket = google_storage_bucket.documents.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${each.value}"
}
