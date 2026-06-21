# Remote state in GCS — PARAMETERIZED, not hardcoded. Supply at init time:
#   terraform init \
#     -backend-config="bucket=YOUR_TFSTATE_BUCKET" \
#     -backend-config="prefix=ei/prod"
# Offline validation (no GCP):  terraform init -backend=false
terraform {
  backend "gcs" {}
}
