# Memorystore for Redis — OPTIONAL until needed (ARCHITECTURE.md §6, CLAUDE.md
# §2). Redis holds only cache/locks/dedupe (rebuildable). Phase 1 can run with
# Postgres advisory locks; enable this when the hot path needs it.
#
# Left commented so it is review-ready but provisions nothing now. To enable:
# uncomment, add a `redis_enabled` var/toggle, and supply an authorized VPC.
#
# resource "google_redis_instance" "cache" {
#   project        = var.project_id
#   name           = "${local.name_prefix}-redis"
#   region         = var.region            # europe-* (data residency)
#   tier           = "BASIC"               # STANDARD_HA for prod
#   memory_size_gb = 1
#   redis_version  = "REDIS_7_0"
#   authorized_network = var.private_network_id
#   transit_encryption_mode = "SERVER_AUTHENTICATION"
#   auth_enabled            = true
#   labels = local.common_labels
# }
