# PRODUCT.md — Product Description

## Problem & goal

Euro Intermed runs a B2B commercial operation across several verticals, qualifying leads manually over WhatsApp, email, and phone. The platform turns that into a digital system: an **AI agent qualifies inbound B2B prospects through natural conversation**, writes structured leads into a system of record, and a consultant works them from a dashboard. Over time the platform accumulates a **proprietary, normalized B2B company database** — the strategic asset that later powers expert matching (SkalYou).

## Actors

- **Prospect / Client** — a business that wants to buy, sell, or (Phase 2) solve a problem. Talks to the agent on the web widget or WhatsApp. No account in Phase 1; Phase-2 clients get a magic-link to view/accept an offer.
- **Consultant / Staff** — internal user who works leads in the dashboard, handles handoffs, tracks offers.
- **Provider / Expert (Phase 2)** — a service provider who receives matched leads, accepts/declines, and uploads a plan + offer. Has an authenticated portal.
- **Admin** — full internal access; manages users/roles. (Admin Global + Country Operator roles are designed-for but implemented later.)

## Verticals

- **Angrosist** (Phase 1) — B2B sourcing. Qualify **buyers**: product, quantity, delivery location, recurring need, deadline, budget, company (CUI).
- **PalletClearance** (Phase 1) — overstock / clearance. Qualify **buyers** (categories, volume, countries, near-expiry tolerance) and **sellers** (stock type, category, quantity, location, expiry, target price, confidentiality, **mandatory photos**).
- **SkalYou** (Phase 2) — business problem-solving: AI + experts + (later) local operators. The agent runs a **diagnostic** conversation, determines the specialist type needed, matches a provider, the provider accepts/declines and uploads a plan + offer, and the client receives it via magic-link.

## Channels

- **Web chat widget** — an embeddable `<script>` for any site, plus a standalone hosted chat page. Real-time (WebSocket/SSE). Our own channel; no external approval needed.
- **WhatsApp** — `wa.me` click-to-chat links on the sites feed the same agent. Per-vertical numbers or intent encoded in the prefilled text. Gated by Meta Business verification.

The **agent core is channel-agnostic** — both channels feed one qualification engine.

## Core capabilities

- **Conversational qualification** — hybrid: a deterministic per-vertical field flow + LLM for understanding, extraction, phrasing, language.
- **Live company verification** — CUI → company data (name, VAT status, administrators, CAEN) and optional financials/turnover, via the free DemoANAF API; results populate the B2B database.
- **B2B company database** — durable companies/contacts/categories, with a `roles[]` classification (distributor, importer, retailer, HoReCa, processor, buyer, seller…) tagged opportunistically — this is the SkalYou provider directory seed.
- **Lead capture & normalization** — every qualified conversation becomes a structured lead + company/contact/request.
- **Deferred human handoff** — escalation sets `needs_human`, mutes the bot for that conversation, and surfaces it in a dashboard queue; staff follow up. (Live takeover is a later add; the `bot_active` flag is built now.)
- **Consultant dashboard** — pipeline, lead detail + transcript, companies (B2B DB), inventory of listings, KPIs, tasks, manual offer tracking, roles, handoff queue.
- **Transactional email** — lead confirmations + internal notifications, with deliverability (SPF/DKIM/DMARC).
- **(Phase 2) Expert matching & marketplace** — diagnostic classification, matching engine, provider portal (Google + email OTP), provider dashboard, client magic-link (view/accept/request-clarifications), monetization **fields**, clickwrap before lead disclosure, multi-country minimal.

## Languages & scope

- RO and EN across conversation, emails, and dashboard.
- Romania is the active market at launch; the data model carries country/market so additional markets are an extension, not a rebuild.

## What the product is NOT (Phase 1/2 boundaries)

- Not a website builder — we ship the widget + WhatsApp links; the sites stay the client's.
- Not a quote generator — manual offer-status tracking only (no PDF quotes, no sending, no e-sign).
- Not a payments/billing system — Phase-2 monetization is **fields and statuses only**, no invoicing/payments.
- No voice messages (text + files only).
- No automated follow-up/nurture sequences (single confirmation + internal notification).
- Phase-2-deferred: full negotiation, commercial multi-country (country operators, currency, VAT/VIES, legal localization), advanced matching (scoring, availability, fan-out), expert tier packages, full audit export.
