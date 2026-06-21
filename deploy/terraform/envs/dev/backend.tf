# Remote state in GCS — PARAMETERIZED, not hardcoded.
#
# The bucket and prefix are NOT written here (they differ per env/owner and the
# bucket must be created once, out-of-band, before the first init). Supply them
# at init time via a partial backend config:
#
#   terraform init \
#     -backend-config="bucket=YOUR_TFSTATE_BUCKET" \
#     -backend-config="prefix=ei/dev"
#
# Or use a gitignored backend.hcl:  terraform init -backend-config=backend.hcl
#
# For offline validation (no GCP), init with:  terraform init -backend=false
terraform {
  backend "gcs" {}
}
