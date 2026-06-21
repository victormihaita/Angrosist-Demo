# UIUX_GUIDE.md — Frontend UI/UX Guide (Euro Intermed B2B Platform)

> Companion to `PRODUCT.md`, `REQUIREMENTS.md` (FR-1 widget, FR-7 dashboard, FR-9 P2 portal/client page), `ARCHITECTURE.md`, root `CLAUDE.md`.
> Audience: frontend devs building the dashboard, widget, and Phase-2 portals screen-by-screen.
> Scope: this is a *build guide*, not component code. It maps every screen to concrete shadcn/ui components and patterns.

---

## 0. Ground rules (the user's hard requirements)

1. **shadcn/ui prebuilt components, no customization.** Use components exactly as the shadcn CLI / MCP installs them. Do **not** hand-edit files in `src/components/ui/` to add bespoke behaviour, do not fork the primitives, do not write parallel custom components that duplicate a shadcn primitive. Compose at the *application* layer (in `src/components/dashboard/`, `src/components/chat/`, pages), never by hacking the primitive.
2. **No custom component CSS hacking.** Styling is done only with **Tailwind utility classes + the design tokens** already defined in `src/index.css` (the shadcn `new-york` / `neutral` theme with CSS variables). No new `.css` files, no `!important`, no inline `style={{…}}` for layout (the legacy widget inline styles are the *only* sanctioned exception — see §10). If a token is missing, add it to the theme variables, do not inline a hex.
3. **Performance first.** Bundle size, route-level code-splitting, lazy widget, TanStack Query caching, and **no fixed-interval polling** (see §3) are non-negotiable, not nice-to-haves.
4. **Clean UX for admins / managers / users.** Predictable layout, consistent density, every async state handled (loading / empty / error), keyboard + screen-reader accessible.

**Pulling components:** add new shadcn primitives via the **shadcn MCP server** (preferred) or `npx shadcn@latest add <name>`. The project is already configured (`components.json`: style `new-york`, baseColor `neutral`, cssVariables `true`, alias `@/components/ui`, lucide icons). Anything the MCP installs lands in `src/components/ui/` and is used as-is.

---

## 1. Design principles

### 1.1 Component sourcing — shadcn only
- The **only** UI primitives are shadcn/ui components in `@/components/ui`. Today the repo has:
  `badge`, `button`, `card`, `dialog`, `input`, `scroll-area`, `separator`, `table`, `textarea`.
- Everything else listed in this guide must be **added via the shadcn MCP** before use. Do not write a primitive by hand when shadcn ships one.
- App-level composites (e.g. `LeadTable`, `LeadDetail`, `EmbedDialog`) live outside `ui/` and are assembled *from* primitives.

### 1.2 Styling & tokens
- Tailwind v4 (`@tailwindcss/vite`) + the shadcn CSS-variable theme. Use semantic tokens, never raw colors:
  `bg-background`, `text-foreground`, `text-muted-foreground`, `bg-card`, `bg-muted`, `border`, `bg-primary`, `text-primary-foreground`, `bg-destructive`, `ring`.
- Status colors come from `Badge` variants (`default` / `secondary` / `destructive` / `outline`), not arbitrary classes. Map domain statuses → variants once in a tiny helper (a pure function, not a component) and reuse.
- Spacing/typography from Tailwind scale only. Keep dashboard density consistent: table rows `h-10`–`h-12`, card padding via shadcn defaults.
- Dark mode: the theme already defines `.dark` variables; honor it, never hardcode light-only colors.

### 1.3 Accessibility (a11y)
- shadcn is built on Radix → keyboard nav, focus traps, ARIA roles come for free **if you use the primitives correctly**. Do not strip Radix props.
- Every icon-only `Button` needs an accessible name (`aria-label` or visually-hidden text). The current `EmbedDialog` copy button is icon-only — give it a label.
- All form inputs go through shadcn `Form` (`FormLabel` + `FormControl` wire `htmlFor`/`aria-describedby` automatically). Never use a bare `<input>` without a label.
- Color is never the sole signal: pair status `Badge` color with text.
- Respect `prefers-reduced-motion` (Tailwind `motion-reduce:` variants) for any animated affordance.
- Target WCAG AA contrast — the `neutral` theme already meets it; don't lower it.
- Dialogs/Sheets must return focus to the trigger on close (Radix default — don't override).

### 1.4 Responsive / mobile-safe
- Layout uses `h-dvh` (already in `App.tsx`) so the virtual keyboard shrinks the viewport instead of pushing the input off-screen. Keep this pattern for any input-at-bottom screen (chat, comment boxes).
- Dashboard breakpoints: `< md` collapses table → stacked cards or horizontal scroll inside a `ScrollArea`; the sidebar/nav collapses into a `Sheet`-based drawer; `md+` shows the full table and persistent nav.
- Dialogs must be mobile-safe: follow the `EmbedDialog` pattern (`w-[calc(100vw-2rem)] max-w-xl`). For full-height mobile flows prefer `Sheet` (side `bottom`) over `Dialog`.
- Touch targets ≥ 44px; use `size="default"`/`size="icon"` (≥ `h-9`/`h-10`), not `size="sm"` for primary mobile actions.

### 1.5 Performance budget
- **Initial dashboard JS (gzipped) target: ≤ 200 KB; landing/chat ≤ 120 KB.** Measure with `vite build` + `rollup-plugin-visualizer` in CI; fail the build if a route bundle regresses materially.
- **Route-level code-splitting:** every page in `src/pages/` is loaded via `React.lazy` + `Suspense` (see §7.2). Public pages (landing/chat) must not pull in dashboard/table code, and vice-versa.
- **Lazy-load the widget:** the embeddable widget is a separate build (`vite.widget.config.ts`, `build:widget`) and is **never** part of the dashboard/app bundle. On the public app, mount the floating widget only after first paint / idle (`requestIdleCallback`), and only on the landing page (the repo already hides it on nav away).
- **TanStack Query caching:** one `QueryClient` with sane defaults — `staleTime` 30–60 s for list/detail reads, `gcTime` 5 min, `refetchOnWindowFocus: true` for freshness without polling. Detail queries keyed `['lead', id]` so navigation reuses cache.
- **No fixed-interval polling.** The current `useLeads` uses `refetchInterval: 30_000` — **remove it.** Replace live updates with **SSE or WebSocket** (FR-1.3 mandates real-time transport for chat; reuse it for the dashboard): a single SSE/WS subscription invalidates the relevant Query keys (`queryClient.invalidateQueries`) when the backend pushes a `lead.created` / `lead.updated` / `handoff` event. Window-focus refetch covers the gap until WS/SSE lands; polling does not return.
- Defer heavy deps: load `recharts`/`chart` only on the KPI route; load `@tanstack/react-table` only on table routes.
- Use `<img loading="lazy">` for any GCS images (seller photos, offers) and request signed URLs on demand.
- Memoize table column defs and row data; virtualize only if a list realistically exceeds ~200 rows (prefer server-side pagination instead — see §4.2).

---

## 2. Information architecture / screen map

### 2.1 Audiences & route groups
| Audience | Auth | Route prefix | Phase |
|---|---|---|---|
| Public prospect | none | `/`, `/chat`, embedded widget | P1 (exists) |
| Staff / Consultant | session | `/dashboard/*` | P1 |
| Admin | session + role | `/admin/*` | P1 |
| Provider / Expert | Google + email OTP | `/portal/*` | P2 |
| Client (offer recipient) | magic-link token | `/offer/:token` | P2 |

### 2.2 Public (already built — keep, do not rebuild)
- **Landing page** — `src/pages/LandingPage.tsx`. Hero + CTA into chat; hosts the floating widget on this page only.
- **Standalone chat page** — `src/pages/ChatPage.tsx` (`/chat`). Full-screen agent chat (FR-1.2). Real-time transport (FR-1.3) — currently request/response `POST /api/chat`; upgrade to SSE/WS with typing indicator.
- **Embeddable widget** — `frontend/widget/` (`WidgetApp.tsx`, `widget-entry.tsx`), built separately to `widget.js`. Floating button → panel (FR-1.1). See §10.

These exist and work; this guide treats them as the baseline to stay consistent with, not greenfield.

### 2.3 Staff / Consultant dashboard (P1)
Shared shell: persistent left **nav** (or top `Nav` as today) + content area. Add a collapsible sidebar for the dashboard section using `Sheet` (mobile) + a static aside (`md+`). Global elements: language switch (§6), user menu (`DropdownMenu` + `Avatar`), `Sonner` toaster mounted once at the root.

| # | Screen | Route | Purpose (FR) |
|---|---|---|---|
| S1 | **Lead pipeline** | `/dashboard` | FR-7.1 — all leads as table (default) with optional Kanban view by status |
| S2 | **Lead detail** | `/dashboard/leads/:id` | FR-7.1/7.3/6.2 — transcript, extracted fields, typed request, company verification panel, offer tracking, activity log |
| S3 | **Companies / B2B directory** | `/dashboard/companies` | FR-4.4/7.2 — company list |
| S4 | **Company detail** | `/dashboard/companies/:id` | FR-4 — `roles[]`, verification, financials |
| S5 | **Listings inventory** | `/dashboard/listings` | FR-7.2 — PalletClearance listings (incl. seller photos) |
| S6 | **Handoff queue** | `/dashboard/handoffs` | FR-6.2 — conversations with `needs_human` |
| S7 | **KPIs dashboard** | `/dashboard/kpis` | FR-7.4 — offers sent, conversion, pipeline value |

> Note: the demo currently routes lead detail at `/dashboard/:id` (`LeadDetailPage.tsx`). For P1, move to the explicit `/dashboard/leads/:id` namespace above so `companies`, `listings`, etc. don't collide with the `:id` param.

#### S1 — Lead pipeline
- **Regions:** filter/search bar → toggle Table/Kanban → data view → pagination.
- **Table view** columns: status `Badge`, vertical `Badge`, company, product/request summary, assigned-to (`Avatar` + name), value, created (relative). Row click → S2. Row `DropdownMenu` for actions (assign, change status, open).
- **Kanban view** (optional, lazy): columns per status, each card = shadcn `Card` (company, value, vertical badge). Drag-drop is *not* required for P1 — status change via the card menu is enough; only add DnD if explicitly requested (it adds bundle + a11y cost).
- **Filters:** status, vertical, assigned, date range, free-text search — all reflected in the URL (§7.3) and sent to the server (§4.2).

#### S2 — Lead detail
- **Layout:** `md+` two-column (main + right rail); mobile stacks. Use `Tabs` for the main column to keep the page light: *Transcript* | *Request* | *Activity*.
  - **Header:** company name, status `Badge`, vertical `Badge`, assigned `Avatar`, primary actions (`Button`s: Change status, Assign, Send offer-status update).
  - **Transcript tab:** `ScrollArea` of message bubbles (reuse the chat `MessageList` styling); role-tagged; render `tool_calls` as collapsible blocks.
  - **Request tab:** extracted fields + typed request (`sourcing_request` for Angrosist / `listing` for PalletClearance) as a definition list inside a `Card`; editable fields go through `Form`.
  - **Activity tab:** `activity_logs` timeline (FR-5.3) — list with `Separator`s, relative timestamps.
  - **Right rail (Cards):**
    - **Company verification panel** — DemoANAF result (name, VAT status, administrators, CAEN), financials if present; `Badge` for verified/unverified; "Re-verify" `Button`. Foreign/unverifiable companies show an empty/optional state, never an error.
    - **Offer tracking** (FR-7.3) — status `Select` (requested → sent → negotiation → won/lost), value `Input`, note `Textarea`, save `Button`; success `toast`; optimistic update (§4.5).
    - **Handoff** — `bot_active` indicator + "Take over / Resolve" action when `needs_human`.

#### S3 — Companies / B2B directory
- Search + filter (role, verification status, county/market) → `DataTable`: company, CUI/reg-no, `roles[]` as multiple `Badge`s, VAT status, county, last verified. Server-side pagination. Row → S4.

#### S4 — Company detail
- Header card (name, reg-no, `roles[]` badges, VAT `Badge`).
- `Tabs`: *Overview* (CAEN, administrators, address) | *Financials* (turnover/financials table, empty state if null) | *Leads* (this company's leads, links to S2) | *Verification history* (`company_verifications` list).
- Role tagging UI: multi-select (`Command` inside a `Popover`, or shadcn combobox) to add/remove `roles[]`; additive only.

#### S5 — Listings inventory (PalletClearance)
- `DataTable`: type (buyer/seller), category, quantity, location, expiry, target price, confidentiality `Badge`, status.
- Detail/drawer (`Sheet`) showing seller **photos** (lazy `<img>` from GCS signed URLs) — emphasize that a seller listing without photos is incomplete (mirrors the blocking rule in the agent flow).

#### S6 — Handoff queue
- Focused list/table of conversations where `needs_human=true`: company/contact, vertical, why-escalated, waiting-since (relative), `bot_active` badge. Row → S2 transcript tab. "Claim" assigns to current user (optimistic + toast).

#### S7 — KPIs dashboard
- Top row: stat `Card`s (offers sent, conversion rate, pipeline value, leads this period).
- `Chart` (shadcn chart / recharts, lazy-loaded): pipeline value over time, conversion funnel, leads by vertical.
- Date-range `Select`/filter in URL state. All numbers from a single aggregated KPI endpoint (don't compute client-side over the full lead list).

### 2.4 Admin (P1)
| # | Screen | Route | Purpose |
|---|---|---|---|
| A1 | **User & role management** | `/admin/users` | create/invite users, assign roles, deactivate |

- `DataTable`: user, email, role `Badge`, status, last active. Row `DropdownMenu`: change role, deactivate, resend invite.
- Invite/edit in a `Dialog` with a `Form` (email, role `Select`). Destructive actions (deactivate) go through `AlertDialog` confirmation.
- Visible only to `admin` (§5).

### 2.5 Provider portal (P2)
| # | Screen | Route | Purpose (FR-9) |
|---|---|---|---|
| P1s | **Onboarding** | `/portal/onboarding` | FR-9.2 — profile, categories/roles/markets, consent |
| P2s | **My leads** | `/portal/leads` | FR-9.2 — own leads only |
| P3s | **Clickwrap gate** | `/portal/leads/:id/terms` | FR-9.3 — accept terms before disclosure |
| P4s | **Lead detail (gated)** | `/portal/leads/:id` | full client data only after clickwrap |
| P5s | **Upload offer** | within P4s | FR-9.2 — accept/decline + upload plan+offer |

- **Auth:** Google + email OTP (FR-9.2). Login screen: `Card` + "Continue with Google" `Button` + OTP `Form` (email → code via `InputOTP`).
- **P1s Onboarding:** multi-step `Form` (use `Tabs` or a stepper composed from `Button`+state): profile, then categories/`roles[]`/markets (`Command` multi-select), then consent checkbox. Cannot finish without consent.
- **P2s My leads:** `DataTable` scoped server-side to the provider; columns: lead ref, category, market, status, received-at. Pre-clickwrap rows show **masked** client data (FR-9.8 anti-circumvention).
- **P3s Clickwrap gate:** full-screen `Card` with terms (in `ScrollArea`), a required `Checkbox` ("I accept commercial terms v{n}"), Accept `Button`. On accept the backend records timestamp/IP/userId/version (FR-9.3) — the UI just posts intent and shows a `toast`. Until accepted, P4s is inaccessible (route guard).
- **P4s Lead detail (gated):** full client data, request, attachments. Two primary `Button`s: **Accept** / **Decline** (Decline → `AlertDialog` + reason). Accepting logs lead-acceptance (FR-9.8).
- **P5s Upload offer:** file upload (plan + offer doc to GCS via signed URL), offer value `Input`, note `Textarea`, submit `Button`; success `toast`; status moves to offer-uploaded (FR-9.5).

### 2.6 Client magic-link page (P2) — no account
- **Route:** `/offer/:token`. Token validated server-side; expired/used token → friendly empty/error state (`Card`, no app chrome). Access is logged (FR-9.4/NFR-2).
- **Single screen, minimal chrome** (no nav, no auth UI):
  - Offer summary `Card` (provider, scope, value, validity).
  - Attachments (download links / lazy preview).
  - Actions: **Accept** `Button` (→ confirmation `AlertDialog`, sets client-accepted, FR-9.5) and **Request clarifications** (`Textarea` in a `Dialog`/`Sheet` → posts message, sets `clarificare solicitată`, notifies, FR-9.4).
  - Confirmation states via `toast` + inline success `Card`.
- Must be the lightest bundle of all surfaces — its own lazy route, no table/chart code.

---

## 3. shadcn component inventory per screen

Legend: **[have]** already in `src/components/ui/` · **[add]** install via shadcn MCP before building.

**Already present:** `badge`, `button`, `card`, `dialog`, `input`, `scroll-area`, `separator`, `table`, `textarea`.

**To add via MCP (P1):** `form`, `label`, `select`, `dropdown-menu`, `tabs`, `skeleton`, `sonner` (toast), `avatar`, `command`, `popover`, `checkbox`, `alert-dialog`, `pagination`, `sheet`, `tooltip`, `chart`. Plus the `data-table` *pattern* (shadcn's data-table recipe = `table` + `@tanstack/react-table`, not a single component — see §4.2).
**To add via MCP (P2):** `input-otp`, `progress` (offer/upload), `radio-group` (accept/decline).

| Screen | shadcn components |
|---|---|
| **Landing** (public) | `button` [have], `card` [have] |
| **Chat page / widget** | `button` [have], `input`/`textarea` [have], `scroll-area` [have], `card` [have], `avatar` [add], `skeleton` [add] (typing/loading); `tooltip` [add] |
| **S1 Lead pipeline** | data-table (`table` [have] + tanstack) , `badge` [have], `button` [have], `input` [have] (search), `select` [add] (filters), `dropdown-menu` [add] (row actions), `pagination` [add], `tabs` [add] (Table/Kanban), `card` [have] (kanban cards), `avatar` [add], `skeleton` [add], `sonner` [add] |
| **S2 Lead detail** | `tabs` [add], `card` [have], `scroll-area` [have], `separator` [have], `badge` [have], `button` [have], `select` [add] (offer/status), `input` [have], `textarea` [have], `form` [add], `label` [add], `avatar` [add], `dropdown-menu` [add], `alert-dialog` [add] (confirm), `sonner` [add], `skeleton` [add], `tooltip` [add] |
| **S3 Companies list** | data-table, `badge` [have] (`roles[]`/VAT), `input` [have], `select` [add], `pagination` [add], `skeleton` [add] |
| **S4 Company detail** | `tabs` [add], `card` [have], `table` [have] (financials), `badge` [have], `command` [add] + `popover` [add] (role multi-select), `separator` [have], `button` [have], `skeleton` [add] |
| **S5 Listings** | data-table, `badge` [have], `sheet` [add] (detail w/ photos), `card` [have], `pagination` [add], `skeleton` [add] |
| **S6 Handoff queue** | data-table / `table` [have], `badge` [have], `button` [have], `avatar` [add], `sonner` [add], `skeleton` [add] |
| **S7 KPIs** | `card` [have] (stats), `chart` [add], `select` [add] (date range), `skeleton` [add], `separator` [have] |
| **A1 Users/roles** | data-table, `dialog` [have], `form` [add], `select` [add], `badge` [have], `dropdown-menu` [add], `alert-dialog` [add], `sonner` [add] |
| **P1s Onboarding** | `card` [have], `form` [add], `input` [have], `select` [add], `command`+`popover` [add], `checkbox` [add], `tabs` [add]/stepper, `button` [have], `sonner` [add] |
| **P2s Provider leads** | data-table, `badge` [have], `select` [add], `pagination` [add], `skeleton` [add] |
| **P3s Clickwrap** | `card` [have], `scroll-area` [have], `checkbox` [add], `button` [have], `sonner` [add] |
| **P4s Gated lead detail** | `card` [have], `tabs` [add], `badge` [have], `button` [have], `alert-dialog` [add], `radio-group` [add], `sonner` [add] |
| **P5s Upload offer** | `form` [add], `input` [have], `textarea` [have], `progress` [add], `button` [have], `sonner` [add] |
| **Provider login (OTP)** | `card` [have], `button` [have], `input-otp` [add], `form` [add], `sonner` [add] |
| **Client offer page** | `card` [have], `button` [have], `alert-dialog` [add], `dialog` [have]/`sheet` [add], `textarea` [have], `sonner` [add] |
| **App shell / nav** | `sheet` [add] (mobile drawer), `dropdown-menu` [add] (user menu), `avatar` [add], `separator` [have], `button` [have] |
| **Empty/loading/error (global)** | `skeleton` [add], `card` [have], `button` [have], `sonner` [add] + a React error boundary (not a shadcn component) |

---

## 4. Patterns

### 4.1 Forms
- **`react-hook-form` + `zod` + shadcn `form`** for every input surface (offer tracking, role editing, admin users, provider onboarding, client clarifications).
  - Install `react-hook-form`, `zod`, `@hookform/resolvers` (add to `package.json`); `form` primitive via MCP.
  - One `zod` schema per form; `zodResolver` wires validation; `Form` + `FormField`/`FormItem`/`FormLabel`/`FormControl`/`FormMessage` render errors accessibly.
  - Submit disabled while `isSubmitting`; on success → `toast.success` + invalidate/optimistic-update the relevant Query; on failure → `toast.error` with the server message.
  - Never trust the client: zod is UX, the Go backend re-validates.

### 4.2 Data tables
- **`@tanstack/react-table` (headless) + shadcn `table`** = the shadcn "data-table" recipe. Build one reusable `<DataTable>` composite in `src/components/dashboard/` and feed it column defs per screen. (Today's `LeadTable.tsx` is the seed — generalize it.)
- **Server-side pagination, filtering, sorting** — match the backend API. The table is *controlled*: page/pageSize/sort/filters live in URL state (§7.3) and are passed as query params; TanStack Query keys include them (`['leads', { page, status, vertical, q }]`) so each filter combo caches independently. Use `keepPreviousData` so paging doesn't flash.
- Columns: define `accessorKey`, `header`, `cell` (render `Badge`/`Avatar`/actions). Memoize column defs.
- Row actions via `DropdownMenu`; bulk actions only if a real workflow needs them.
- Pagination UI: shadcn `pagination` reflecting server `total`/`pageCount`.

### 4.3 Toasts / feedback
- One `Sonner` `<Toaster />` mounted at the app root. Use `toast.success` / `toast.error` / `toast.loading` for every mutation result. No `alert()`, no custom snackbars.

### 4.4 Empty / loading / error states (every async surface)
- **Loading:** shadcn `Skeleton` matching the final layout (table rows, cards, detail rail). No spinner-only screens for primary content; `Suspense` fallback per lazy route is a light skeleton.
- **Empty:** a `Card` with a one-line explanation + primary action (e.g. "No leads yet — share your chat link"). Distinguish "no data" from "no results for this filter" (offer a Clear-filters `Button`).
- **Error:** a React **error boundary** per route (render a `Card` with retry `Button`); query errors surface inline ("Couldn't load leads — Retry" calling `refetch`) plus a `toast`. Verification panel: unverifiable ≠ error — show an optional/empty state.

### 4.5 Optimistic updates
- For fast, low-risk mutations (assign lead, change status, offer-tracking save, claim handoff): TanStack Query `onMutate` → `cancelQueries` → snapshot → optimistic `setQueryData` → `onError` rollback → `onSettled` invalidate. Pair with a `toast`. Use for status/assignment toggles; **not** for irreversible/legal actions (clickwrap accept, offer accept, erasure) — those wait for the server and confirm via `AlertDialog`.

---

## 5. Role-based view rules (RBAC)

Roles (from `PRODUCT.md`/`CLAUDE.md`): **consultant/staff**, **admin** (P1); **provider** (P2); **client** (magic-link, no role). Designed-for: Admin Global, Country Operator (later).

- The frontend **renders to the role but never enforces security** — the Go backend authorizes every request (NFR-2). UI gating is UX only.
- Fetch the current user + role once (`['me']` query); gate routes with a small `<RequireRole roles={[...]}>` wrapper that redirects unauthorized users.
- **Consultant/staff:** all `/dashboard/*` (S1–S7). Can edit leads/offers/handoffs. Sees only data within their market/assignment if the backend scopes it (country fields exist for later). No `/admin/*`.
- **Admin:** everything staff sees **plus** `/admin/users` (A1). Admin-only controls (role change, deactivate) are hidden for non-admins.
- **Provider (P2):** only `/portal/*`, and **only their own leads** (server-scoped, FR-9.2). Full client data hidden until clickwrap accepted (route guard on P4s). Never sees the staff dashboard.
- **Client (P2):** only `/offer/:token`, no nav/menu, no other route reachable. Token is the only credential.
- Hide vs disable: hide actions a role can never perform; disable (with `Tooltip` reason) actions blocked by *state* (e.g. "Accept" disabled until clickwrap).

---

## 6. i18n (RO / EN)

- **RO is the default**, EN available (NFR-1). The UI today is RO-leaning (e.g. EmbedDialog copy) — formalize it.
- Use a lightweight library: **`i18next` + `react-i18next`** (small, lazy-loadable namespaces). Avoid heavier solutions.
- **String organization:** `src/locales/{ro,en}/<namespace>.json`, namespaced by area: `common`, `dashboard`, `leads`, `companies`, `listings`, `kpis`, `admin`, `portal`, `client`, `chat`. Lazy-load namespaces per route so the landing/chat bundle doesn't ship dashboard strings.
- **No hardcoded user-facing strings** in components — every label/button/toast goes through `t('namespace.key')`. Keys are stable identifiers, not English text.
- Language switch in the app shell (`Select`/`DropdownMenu`); persist choice (localStorage) and reflect on `contact.language` where relevant; set `<html lang>`.
- Format dates/numbers/currency with `Intl` (RO and EN locales) — never manual formatting. Pipeline value uses currency from the data, formatted per locale.
- The agent conversation language is separate (driven by the backend, FR-2.6); the dashboard i18n is independent of the chat language.

---

## 7. State management

### 7.1 Server state — TanStack Query (already in use)
- All API data through TanStack Query (already wired in `App.tsx` / `useLeads.ts`). Centralize fetchers in `src/lib/api.ts` and hooks in `src/hooks/`.
- Defaults: `staleTime` 30–60 s, `gcTime` 5 min, `refetchOnWindowFocus: true`, `retry` 1–2 with backoff. **No `refetchInterval`** (remove the existing 30 s) — freshness comes from focus refetch + SSE/WS invalidation (§3).
- Mutations via `useMutation` with optimistic updates (§4.5) and toast feedback.
- Query keys are structured arrays including all filter/page params so caching is correct and invalidation is targeted.

### 7.2 Routing & code-splitting
- `react-router-dom` (already used). Convert page imports to `React.lazy` and wrap `<Routes>` in `<Suspense fallback={<RouteSkeleton/>}>`. Group public vs dashboard vs portal vs client into separate lazy chunks.

### 7.3 URL state for filters
- Table filters, pagination, sort, tab selection, date ranges live in the **URL** (`useSearchParams`), not component state — shareable, back-button-friendly, and the single source of truth feeding Query keys.

### 7.4 No heavy global store
- **Do not add Redux/Zustand/MobX** unless a concrete need appears. Server state = Query; URL state = router; the little remaining UI state (open dialog, current language) = local `useState`/Context. Keep the global surface tiny.

---

## 8. Environment & secrets

- **All API base URLs via `VITE_*` env vars** — never hardcode hosts. `VITE_API_URL` is the contract (already used in `api.ts`, `EmbedDialog.tsx`, `LandingPage.tsx`). Add new ones as needed (e.g. `VITE_WS_URL` for SSE/WS) — all `VITE_`-prefixed.
- The widget reads its API URL from runtime config (`config.apiUrl` / `window.__ANGROSIST_API_URL__`) so a single `widget.js` works across host sites — keep that.
- **No secrets in the client bundle.** Everything `VITE_` is public by definition — only put non-secret config there (API URLs, public keys). LLM keys, WhatsApp tokens, DB creds, email keys live in **Secret Manager** server-side only (NFR-2). Never reference a secret from frontend code.
- `.env` files are not committed; `.env.example` documents the required `VITE_*` vars.
- Provide a typed `src/lib/env.ts` that reads `import.meta.env` once and fails fast (clear console error) if a required var is missing — no scattered `import.meta.env` reads with silent `?? ''` fallbacks that hide misconfig.

---

## 9. Widget specifics (FR-1.1)

- **Separate build/bundle:** `vite.widget.config.ts` → `widget.js`, built via `build:widget`/`build:all`, CSS injected by JS (`vite-plugin-css-injected-by-js`) so it ships as a single drop-in script. It is **never** imported by the dashboard app and vice-versa.
- **Embed contract** (matches `EmbedDialog`):
  ```html
  <script src="https://…/widget.js"></script>
  <script>AngrosistChat.init({ apiUrl: '…' });</script>
  ```
  `init` is idempotent (the existing `mounted` guard); supports floating mode (default, bottom-right) or inline mount via `containerId`.
- **Theming via config:** extend `init(config)` to accept optional `theme` (primary color, position, locale, vertical/intent prefill) — config-driven only, no host-site CSS editing required. Defaults must look correct with zero config.
- **Isolation:** the widget runs on third-party pages, so it must not collide with host styles. The current implementation uses **inline styles** for the floating button/wrapper — keep host-facing chrome isolated. Preferred path: mount the widget inside a **Shadow DOM** root and inject the widget CSS into that shadow root (Tailwind/shadcn styles scoped, immune to host CSS, and host CSS can't bleed in). High `z-index` (already `999999`) and a unique container id (`__angrosist_widget__`) stay.
- **Performance:** the widget is the most size-sensitive surface — strict small-bundle budget. Inside it, use only the minimal shadcn primitives needed (button, input/textarea, scroll-area). Defer mounting until idle; load the heavy chat panel only when the user opens the bubble (lazy the panel, ship just the button initially). Real-time via SSE/WS (FR-1.3) with a typing indicator. No analytics/tracking weight.
- **A11y on host sites:** the launcher button needs an `aria-label` (e.g. "Open chat"), focus management when the panel opens/closes, and `Esc` to close.

---

## 10. Migration notes vs current code (quick wins for the dev)

1. Remove `refetchInterval: 30_000` from `useLeads.ts`; add focus-refetch defaults to the `QueryClient`; add SSE/WS invalidation when the transport lands.
2. Generalize `LeadTable.tsx` into a reusable server-paginated `DataTable` (tanstack-react-table) and drive all tables from it.
3. Namespace dashboard routes (`/dashboard/leads/:id`, `/companies`, `/listings`, `/handoffs`, `/kpis`) instead of the demo's `/dashboard/:id`.
4. Lazy-load all pages (`React.lazy` + `Suspense`) and split public vs dashboard chunks.
5. Add `Sonner` toaster at root; route every mutation result through it.
6. Introduce i18next with namespaced RO/EN strings; replace inline RO strings.
7. Add the `VITE_*` env accessor with fail-fast validation.
8. Add the missing shadcn primitives via the MCP **before** building each screen (see §3).
9. Move the widget chrome to a Shadow DOM root for isolation; keep `init` idempotent + config-driven.
```
