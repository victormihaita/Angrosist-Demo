# Cloudflare — DNS / TLS / WAF / CDN in front of the public api + widget
# (ARCHITECTURE.md §6, SECURITY.md §9.2). OPTIONAL / documented stub.
#
# Cloudflare is a SEPARATE provider (requires a Cloudflare API token in its own
# secret, never committed) and a separate zone. Kept commented so this root
# stays GCP-only and provisions nothing until DNS is ready. To enable: add the
# cloudflare provider to versions.tf, supply zone_id + token via env, and point
# a CNAME at the Cloud Run / load-balancer hostname.
#
# terraform {
#   required_providers {
#     cloudflare = { source = "cloudflare/cloudflare", version = "~> 4.0" }
#   }
# }
#
# resource "cloudflare_record" "api" {
#   zone_id = var.cloudflare_zone_id
#   name    = var.api_hostname           # e.g. api.staging.example.com
#   type    = "CNAME"
#   content = module.run_api.uri         # Cloud Run managed hostname / LB
#   proxied = true                       # WAF + CDN + TLS termination
# }
#
# Also configure: WAF rules, rate limiting on /api/chat and the WhatsApp webhook
# (SECURITY.md §1.1 / §1.2), and keep EU traffic in-region (§7.5).
