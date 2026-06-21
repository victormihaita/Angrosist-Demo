# Map of logical secret key -> created secret_id, for wiring into Cloud Run
# secret env. Values are NEVER output (none exist in TF state).
output "secret_ids" {
  description = "Logical secret key to fully-qualified Secret Manager secret_id."
  value       = { for k, s in google_secret_manager_secret.this : k => s.secret_id }
}
