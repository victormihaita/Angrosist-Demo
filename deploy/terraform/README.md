# Terraform — Euro Intermed GCP infrastructure

Infrastructure-as-code for the Euro Intermed B2B platform. All resources run on
GCP in **EU regions only** (`europe-*`), per the EU data-residency requirement
(`docs/specs/SECURITY.md` §7.5).

> **Status: build cloud-ready, provisioning DEFERRED.** Nothing here is applied
> yet. `terraform validate` works fully offline. `plan`/`apply` require GCP
> projects + credentials that do not exist yet. **Deploys come from CI, never a
> laptop** (`CLAUDE.md` §6). The steps below are what the owner runs *later*,
> once GCP is ready.

## Layout

```
deploy/terraform/
  modules/
    artifact_registry/    Docker repo (image scanning, cleanup policy)
    iam_service_accounts/ api / worker / deployer SAs + least-privilege project roles
    secret_manager/       7 secret RESOURCES by name (no values) + per-secret accessor IAM
    cloud_sql/            Postgres, EU, automated backups + PITR, private IP, IAM auth
    gcs/                  documents bucket: EU, uniform access, versioning, signed-URL-only
    cloud_run_service/    reusable Cloud Run v2 service (used for api and worker)
    cloud_tasks/          async agent-turn queue (push target = worker)
    cloud_scheduler/      cron job placeholders (timeouts/reminders/[P2] matching)
    platform/             composition module wiring all of the above for ONE env
                          (+ optional_redis.tf / optional_cloudflare.tf stubs)
  envs/
    dev/      staging/      prod/     thin roots: backend + provider + platform module
  README.md
```

Each environment is a **separate GCP project** with **separate Terraform state**
(`CLAUDE.md` §6). Modules are reusable; per-env differences are variables only,
no duplication.

## Secrets & identity model

- **No secret values anywhere in this code or in state-committed-to-git.** The
  `secret_manager` module creates seven *empty* secret containers by name:
  `db-password`, `llm-api-key`, `whatsapp-token`, `whatsapp-app-secret`,
  `whatsapp-verify-token`, `email-api-key`, `jwt-secret` (prefixed per env, e.g.
  `ei-staging-llm-api-key`). Values are added **out-of-band** (step 4 below).
- Cloud Run reads secrets **by reference** (`secret_env` -> `latest` version);
  values never pass through Terraform.
- Each secret's `secretAccessor` role is granted **only** to the api + worker
  runtime SAs (least privilege, scoped by secret name).
- The one DB password Terraform needs (to create the SQL user) is passed at
  apply time via `-var` from Secret Manager — never written to a `.tfvars` file.
- **CI authenticates via Workload Identity Federation / OIDC** and impersonates
  the per-env `deployer` SA. No exported JSON keys exist.

## State backend (parameterized, not hardcoded)

`backend "gcs" {}` is declared empty in each env. Supply the bucket + prefix at
init time so nothing project-specific is committed:

```bash
terraform -chdir=deploy/terraform/envs/staging init \
  -backend-config="bucket=YOUR_TFSTATE_BUCKET" \
  -backend-config="prefix=ei/staging"
```

Or keep a gitignored `backend.hcl` per env and run `init -backend-config=backend.hcl`.

## Offline validation (no GCP needed — safe now)

```bash
terraform fmt -recursive deploy/terraform

for env in dev staging prod; do
  terraform -chdir=deploy/terraform/envs/$env init -backend=false
  terraform -chdir=deploy/terraform/envs/$env validate
done
```

This never contacts GCP and never applies anything.

---

## Provisioning steps the owner runs LATER (when GCP is ready)

Do this **per environment** (`dev`, then `staging`, then `prod`).

1. **Create the GCP project** (one per env) and set billing.
   ```bash
   gcloud projects create EI_STAGING_PROJECT_ID
   gcloud billing projects link EI_STAGING_PROJECT_ID --billing-account=XXXX
   ```

2. **Enable the required APIs** in that project:
   ```bash
   gcloud services enable \
     run.googleapis.com artifactregistry.googleapis.com sqladmin.googleapis.com \
     secretmanager.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
     storage.googleapis.com iam.googleapis.com iamcredentials.googleapis.com \
     servicenetworking.googleapis.com compute.googleapis.com \
     --project EI_STAGING_PROJECT_ID
   ```

3. **Create the Terraform state bucket** (once; EU, versioned):
   ```bash
   gcloud storage buckets create gs://YOUR_TFSTATE_BUCKET \
     --project EI_STAGING_PROJECT_ID --location=EU --uniform-bucket-level-access
   gcloud storage buckets update gs://YOUR_TFSTATE_BUCKET --versioning
   ```

4. **Create the secret VALUES out-of-band** (Terraform only made the
   containers). Example for the DB password:
   ```bash
   printf '%s' "$(openssl rand -base64 32)" | \
     gcloud secrets versions add ei-staging-db-password --data-file=- \
       --project EI_STAGING_PROJECT_ID
   ```
   Repeat for `llm-api-key`, `whatsapp-*`, `email-api-key`, `jwt-secret`.
   > The secret *containers* are created by `terraform apply`, so on the very
   > first apply: run apply once to create them, then add versions, then the
   > services pick up `latest`. (Cloud SQL user needs the DB password value at
   > apply time — generate it first and pass via `-var`, then store the same
   > value as the `db-password` secret version.)

5. **Configure Workload Identity Federation** so GitHub Actions can impersonate
   the `deployer` SA without keys (see `.github/workflows/backend.yml` header for
   the exact `gcloud iam workload-identity-pools` commands and the GitHub
   secrets/vars to set).

6. **Init + plan + apply** the environment:
   ```bash
   cd deploy/terraform/envs/staging
   cp terraform.tfvars.example terraform.tfvars   # fill project_id, region, CORS
   terraform init -backend-config="bucket=YOUR_TFSTATE_BUCKET" -backend-config="prefix=ei/staging"
   terraform plan  -var "db_password=$(gcloud secrets versions access latest --secret=ei-staging-db-password)"
   terraform apply -var "db_password=$(gcloud secrets versions access latest --secret=ei-staging-db-password)"
   ```

7. **Hand the outputs to CI** (registry base, SA emails, Cloud SQL connection
   name, bucket, queue) as GitHub Actions variables/secrets so the deploy
   workflow can push + deploy.

## Per-environment differences (variables only)

| | dev | staging | prod |
|---|---|---|---|
| deletion protection | off | on | on |
| Cloud SQL tier | f1-micro | custom-1-3840 | custom-2-7680 |
| DB availability | ZONAL | ZONAL | REGIONAL (HA) |
| backups retained | 7 | 14 | 30 |
| api min instances | 0 | 1 | 2 |
| worker min instances | 0 | 0 | 1 |

PITR, EU region, private-IP DB, signed-URL-only bucket, versioning, and
least-privilege SAs are identical across all environments.

## Optional / deferred

- **Memorystore Redis** — commented in `modules/platform/optional_redis.tf`.
  Phase 1 uses Postgres advisory locks; enable Redis when the hot path needs it.
- **Cloudflare DNS/WAF/CDN** — commented in
  `modules/platform/optional_cloudflare.tf`. Separate provider + token; enable
  when DNS is ready.
- **VPC / private networking** — pass `private_network_id` to enable Cloud SQL
  private IP end-to-end. Until a VPC exists, the DB is created without a public
  IP (no public exposure) and reached via the Cloud SQL connector.

## Worker entrypoint note

The backend `Dockerfile` currently builds `server` + `migrate` only; the
`worker` binary lands in M2. Both Cloud Run services default to `/app/server`.
When the image builds `/app/worker`, set `worker_container_command =
["/app/worker"]` in each env's `main.tf` (already present, commented). No module
change needed.
