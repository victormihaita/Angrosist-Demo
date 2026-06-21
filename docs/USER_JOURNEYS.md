# USER_JOURNEYS.md — What the platform does, per user type

> A plain-language walkthrough of every actor's journey through the Euro Intermed B2B platform, so the **final outcome** is unambiguous. Phase tags: `[P1]` Phase 1 · `[P2]` Phase 2 (SkalYou) · `[P3]` Phase 3 (Country Operator).
>
> The one-sentence outcome: **an AI agent qualifies inbound B2B prospects through natural conversation on any channel, verifies their company live, turns each chat into a structured lead, and feeds a growing B2B company database that consultants work in a dashboard — and that later powers expert matching.**

---

## The actors at a glance

| Actor | Has an account? | Where they live | Core goal |
|---|---|---|---|
| **Buyer prospect** (Angrosist) `[P1]` | No | Web widget / WhatsApp | Get matched to wholesale supply |
| **Buyer prospect** (PalletClearance) `[P1]` | No | Web widget / WhatsApp | Subscribe to a clearance/overstock feed |
| **Seller prospect** (PalletClearance) `[P1]` | No | Web widget / WhatsApp | List stock for sale (with photos) |
| **Client** (SkalYou) `[P2]` | No (magic-link) | Web widget / WhatsApp + email link | Get an expert to solve a business problem |
| **Consultant / Staff** `[P1]` | Yes (login) | Dashboard | Work leads, track offers, handle handoffs |
| **Admin** `[P1]` | Yes (login) | Dashboard | Manage users, roles, settings |
| **Provider / Expert** `[P2]` | Yes (Google + OTP) | Provider portal | Accept matched leads, upload offers |
| **Country Operator** `[P3]` | Yes (scoped) | Dashboard (country-scoped) | Run a single country's operation |

The **AI agent** is not an actor — it's the engine every prospect talks to, the same core across all channels.

---

## 1. Buyer prospect — Angrosist `[P1]` (the flagship flow)

**Who:** a Romanian business that wants to buy goods wholesale.

**Journey:**
1. Lands on a partner site with the embedded chat widget (or clicks a `wa.me` WhatsApp link) — **no signup**.
2. The agent greets them in their language (RO/EN, auto-detected) and asks what they need.
3. Through natural conversation the agent collects: **product, quantity, unit, delivery location, recurring?, deadline, budget, company CUI, phone, email** — one or two questions at a time, never a rigid form.
4. The moment it has the **CUI**, the agent verifies the company **live against the Romanian registry (DemoANAF)** — pulls the legal name, VAT status, CAEN, administrators. If the CUI is wrong, it asks again; if the registry is down, it says "we'll verify manually" and continues.
5. The agent gives consent/privacy notice and captures consent.
6. It summarizes the request, confirms, and submits — creating a **lead + company + contact + sourcing request** in our system.
7. The prospect gets a **confirmation email**; staff get an **internal notification**.
8. **Outcome:** the prospect is qualified in minutes without a form or a phone call; the consultant receives a clean, verified lead.

**Escape hatch:** at any point the prospect can ask for a human → the agent hands off (mutes itself, queues the lead for staff).

---

## 2. Buyer prospect — PalletClearance `[P1]`

**Who:** a business that buys overstock / near-expiry / clearance pallet lots.

**Journey:** same channel + agent. The agent collects **product categories, volume capacity, target countries, near-expiry tolerance**, and whether they want to **subscribe to the clearance feed**. On submit it creates a **lead + a standing buyer profile** so they can be notified of matching lots later.
**Outcome:** the buyer is on record with a reusable demand profile, not just a one-off lead.

---

## 3. Seller prospect — PalletClearance `[P1]`

**Who:** a business with stock to clear.

**Journey:** the agent collects **stock type, category, quantity, location, expiry, target price, confidentiality**. Crucially, it asks for **photos of the stock — and will not let the listing be submitted until at least one photo is uploaded** (photos go to secure cloud storage). On submit it creates a **lead + a listing** in the inventory.
**Outcome:** a photo-backed, qualified listing that staff (and later buyers) can act on. The mandatory-photo gate guarantees listing quality.

---

## 4. Client — SkalYou `[P2]` (the marketplace flow)

**Who:** a business with a problem to solve (e.g. enter a new market, fix logistics, tax/e-commerce help).

**Journey:**
1. Talks to the agent, which runs a **diagnostic conversation** — extracts country, industry, urgency, estimated value, and the **type of specialist** needed, then classifies the need (or escalates to staff if it can't).
2. The platform **matches the best provider** for that need (role + category + market) and routes the lead to them.
3. Later, the client receives a **magic-link email** (no account, no password). Clicking it — within the link's expiry — lets them **view the provider's plan + offer, accept it, or request clarifications**. Every view/action is logged.
**Outcome:** the client gets expert help without ever creating an account; the platform brokered the match and the offer.

---

## 5. Consultant / Staff `[P1]` (the people who run the business)

**Who:** internal Euro Intermed staff.

**Journey (the daily dashboard):**
- **Pipeline:** see all leads with status, vertical, assigned owner, value, date — filter and search.
- **Lead detail:** read the full **chat transcript**, the **extracted fields**, the **typed request**, and the **company verification panel** (registry data) side by side.
- **Companies / B2B directory:** browse the growing company database with roles, verification, and financials — the strategic asset.
- **Listings inventory:** PalletClearance stock.
- **Offer tracking:** move an offer through requested → sent → negotiation → won/lost, with value and notes (manual, no quoting engine).
- **Handoff queue:** pick up conversations the agent escalated, with full context.
- **KPIs:** offers sent, conversion rate, pipeline value.
**Outcome:** staff spend time closing deals, not qualifying or data-entry — the agent did that.

---

## 6. Admin `[P1]`

**Who:** the platform owner / manager.

**Journey:** everything staff can do, **plus** manage users and roles, and platform settings. Controls who has access to what.
**Outcome:** the business can run itself operationally without developer involvement.

---

## 7. Provider / Expert `[P2]`

**Who:** an external specialist who fulfills SkalYou leads.

**Journey:**
1. Signs up via **Google + email OTP**, onboards a profile (categories, roles, markets, consent).
2. Gets matched leads in a **provider dashboard**.
3. Before seeing the client's identity/details, must **accept commercial terms via clickwrap** (timestamp/IP/version recorded) — this protects against lead leakage.
4. **Accepts or declines** the match; if accepted, **uploads a plan + offer**.
5. If a provider doesn't respond in time, the platform automatically re-routes to the next best provider (up to 3 tries, then escalates to staff).
**Outcome:** experts get a steady stream of qualified, matched leads; the platform keeps control of the client relationship and the commission record.

---

## 8. Country Operator `[P3]`

**Who:** a partner running the platform in a specific country.

**Journey:** a **country-scoped** version of the staff dashboard — sees only their country's leads, providers, and routing; basic localization for their market.
**Outcome:** the platform expands to new markets without rebuilding — the data model already carries country/market everywhere.

---

## How it all connects (the big picture)

```
 Prospect (any channel, any vertical)
        │  natural conversation
        ▼
   AI agent  ──verifies company──►  Romanian registry (DemoANAF)
        │  extracts + structures
        ▼
   Lead + Company + Contact + typed request   ──►  B2B company database (the asset)
        │                                              │
        ▼                                              ▼
   Consultant dashboard  ◄──notify── email      Phase 2: expert matching
   (pipeline, offers, handoffs, KPIs)                  │
                                                       ▼
                                          Provider portal + client magic-link
```

Every conversation, on every channel, in every vertical, flows through **one agent core** and lands in **one normalized database**. Phase 1 monetizes consultant-run deals; Phase 2 turns the accumulated company database into a matching marketplace; Phase 3 scales it country by country. That is the final outcome.
