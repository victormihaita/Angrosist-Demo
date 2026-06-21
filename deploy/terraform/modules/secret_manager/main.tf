# Secret Manager — declares secret RESOURCES by name only (SECURITY.md §2).
#
# CRITICAL: this module never contains secret VALUES. Terraform creates the
# empty secret containers; the actual versions (values) are added out-of-band
# (gcloud / console / a separate value-injection step in CI) so secrets never
# touch Terraform state or git. Cloud Run references these by name via the
# cloud_run_service module's secret env mapping.
#
# Accessor IAM is scoped PER SECRET to exactly the service account(s) that need
# it (least privilege, SECURITY.md §2.2 / §9.3): the api SA and the worker SA
# read application secrets; no broad project-level Secret Manager role is granted.

locals {
  # The full secret inventory (SECURITY.md §2.1). Each is created empty.
  # `accessors` lists which runtime SAs may read that secret.
  secrets = {
    db-password = {
      description = "Cloud SQL database password (referenced by DATABASE_URL assembly)."
      accessors   = var.app_accessor_sa_emails
    }
    llm-api-key = {
      description = "LLM provider API key (Gemini for M0 demo, Anthropic for P1+)."
      accessors   = var.app_accessor_sa_emails
    }
    whatsapp-token = {
      description = "WhatsApp Cloud API access token."
      accessors   = var.app_accessor_sa_emails
    }
    whatsapp-app-secret = {
      description = "WhatsApp app secret for X-Hub-Signature-256 verification."
      accessors   = var.app_accessor_sa_emails
    }
    whatsapp-verify-token = {
      description = "WhatsApp webhook GET verify-token challenge value."
      accessors   = var.app_accessor_sa_emails
    }
    email-api-key = {
      description = "Transactional email provider API key (EU region account)."
      accessors   = var.app_accessor_sa_emails
    }
    jwt-secret = {
      description = "JWT/HMAC signing secret for sessions and P2 magic-links."
      accessors   = var.app_accessor_sa_emails
    }
  }

  # Flatten {secret -> [sa,...]} into discrete (secret, sa) IAM bindings.
  secret_accessors = flatten([
    for secret_id, cfg in local.secrets : [
      for sa in cfg.accessors : {
        key       = "${secret_id}:${sa}"
        secret_id = secret_id
        sa        = sa
      }
    ]
  ])
}

resource "google_secret_manager_secret" "this" {
  for_each = local.secrets

  project   = var.project_id
  secret_id = "${var.name_prefix}-${each.key}"

  # Pin replication to EU regions for data residency (SECURITY.md §7.5).
  replication {
    user_managed {
      dynamic "replicas" {
        for_each = var.replica_regions
        content {
          location = replicas.value
        }
      }
    }
  }

  labels = merge(var.labels, { secret = each.key })
}

# Per-secret accessor grant — least privilege, scoped by secret name.
resource "google_secret_manager_secret_iam_member" "accessor" {
  for_each = { for a in local.secret_accessors : a.key => a }

  project   = var.project_id
  secret_id = google_secret_manager_secret.this[each.value.secret_id].secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${each.value.sa}"
}
