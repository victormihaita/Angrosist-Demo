# DATA_MODEL.md — Data Model

Design philosophy: a `lead` is a **thin event** that points at a **typed request**. Durable B2B entities (companies, contacts, categories) are the asset. New verticals and Phase-2 features are **additive** — new sibling tables and nullable columns, never migrations that reshape existing data.

## Core entities (Phase 1)

### companies
The B2B asset. Dedup key **(country, reg_no)** — generalizes beyond Romanian CUI to foreign companies.
- `id`, `country`, `reg_no` (CUI for RO), `name`, `vat_status`, `caen`, `address`
- `roles[]` — nullable classification: distributor, importer, wholesaler, retailer, horeca, processor, producer, buyer, seller. **Tagged opportunistically; seeds the Phase-2 provider directory.**
- timestamps

### company_verifications  (nullable; RO/DemoANAF-specific)
- `company_id`, `source` (demoanaf), `vat_status`, `administrators`, `raw` (jsonb), `checked_at`

### company_financials  (nullable; optional)
- `company_id`, `year`, `turnover`, `raw` (jsonb)  — from DemoANAF `/financials`

### contacts
- `id`, `company_id`, `name`, `phone`, `email`, `language` (ro/en), `consent_id`

### categories
Shared taxonomy (hierarchical) used by all verticals — the matching enabler.
- `id`, `parent_id`, `name`, `code`

### leads  (thin event)
- `id`, `contact_id`, `company_id`, `assigned_to` (user), `vertical` (enum, extensible), `intent` (enum, extensible), `source` (channel/site/number), `status` (enum), `summary`, `needs_human` (bool), `bot_active` (bool), timestamps
- Points at exactly one typed request below.

### sourcing_requests  (Angrosist buyer demand)
- `lead_id`, `category_id`, `product`, `quantity`, `delivery_location`, `recurring` (bool), `deadline`, `budget`

### listings  (PalletClearance supply / clearance)
- `lead_id`, `company_id`, `category_id`, `stock_type`, `quantity`, `location`, `country`, `expiry`, `target_price`, `confidential` (bool), `status`

### buyer_profiles  (standing demand, e.g. PalletClearance buyer feed)
- `company_id`, `vertical`, `categories[]`, `countries[]`, `near_expiry_ok` (bool), `subscribed` (bool)

### documents  (polymorphic)
- `id`, `owner_type` (lead/listing/offer…), `owner_id`, `kind` (photo/product_list/offer), `gcs_key`, `mime`, `created_at`

### consents
- `id`, `contact_id`, `text_version`, `given_at`, `channel`, `ip`

### activity_logs / audit
- `id`, `actor_type` (agent/staff/provider/system), `actor_id`, `action`, `entity_type`, `entity_id`, `meta` (jsonb), `at`

### users & roles
- `users`: `id`, `email`, `name`, `role`
- roles (P1): `staff`, `admin`. **Designed to add** (P2): `provider/expert`, and later `admin_global`, `country_operator`.

### templates
- email/message templates, RO/EN.

## Enums (extensible — never hardcode the full set)

- `vertical`: `angrosist`, `palletclearance`, **[P2]** `skalyou`
- `intent`: `buy`, `sell`, **[P2]** `market_entry`
- `lead.status` (P1): `new`, `qualifying`, `needs_human`, `qualified`, `offer_requested`, `offer_sent`, `negotiation`, `won`, `lost`
- `company.roles[]`: distributor, importer, wholesaler, retailer, horeca, processor, producer, buyer, seller

## Phase 2 additions (additive — schema already allows)

### market_entry_requests  (SkalYou — third sibling of lead's typed request)
- `lead_id`, `company_id`, `category_id`, `partner_types[]` (distributor/buyer/importer/retail/horeca/processor), `has_permanent_stock` (bool), `active_markets[]` (countries), `target_market`, `website`, `objective` (text), `specialist_type`

### providers  (SkalYou experts; extends companies/users)
- `id`, `company_id`, auth identity (Google/OTP), `categories[]`, `roles[]`, `markets[]`, `plan` (starter/pro/vip), `consent_to_leads`, `quality_score` (nullable, P2-later)

### matches
- `id`, `request_id`, `provider_id`, `rank`, `status` (routed/accepted/declined/expired), `routed_at`, `responded_at`, `attempt_no`

### offers
- `id`, `lead_id`/`request_id`, `provider_id`, `document_id` (GCS), `value`, `status` (uploaded/delivered/accepted/clarification_requested), `delivered_at`, `client_action_at`

### lead_terms_acceptance  (clickwrap / anti-leakage)
- `id`, `lead_id`, `provider_id`, `terms_version`, `accepted_at`, `ip`, `user_id`

### monetization fields (on offer/match/transaction — data only, no billing)
- `commission` (applicable %), `offer_value`, `estimated_commission`, `transaction_status` (uninvoiced/invoiced/paid/cancelled), `lead_accepted_at`

### multi-country fields (minimal)
- `country` on client/expert; `target_market`; `market_tags[]`; used for dashboard filtering + matching. RO active at launch; currency/VAT/VIES/legal localization are **later**.

## Design rules (invariants)

1. Dedup companies on **(country, reg_no)**.
2. `vertical` / `intent` are extensible enums.
3. Verification/financials are **nullable** (foreign companies, or not-yet-verified).
4. `companies.roles[]` is populated opportunistically across all verticals.
5. A `lead` points at one typed request; adding a vertical = adding a sibling table (no migration of existing data).
6. Files always in GCS (`documents.gcs_key`), never on disk.
7. Consent + audit are first-class; erasure cascades contact→lead→conversation→files.
8. Migrations forward-only; additive by default.
