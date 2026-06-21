# frontend/CLAUDE.md — React/TypeScript conventions

> Layer-specific rules for the frontend. Inherits all **Hard rules** from the root `CLAUDE.md`.
> Stack: React 19 + TypeScript + Vite. Deploy: Firebase Hosting / Cloud Run (prod), Vercel (demo).
> Full screen-by-screen guidance: `docs/specs/UIUX_GUIDE.md`.

## What lives here

- **Dashboard** (staff/admin; P2 provider portal + client magic-link page).
- **Public**: landing page, standalone chat page.
- **Embeddable widget** (`frontend/widget/`) — separate Vite build (`vite.widget.config.ts`) → small, isolated, inline-styled bundle mountable on any site.

The frontend is **independently deployable** and talks to the backend only over the documented API (`docs/specs/openapi.yaml`).

## UI: shadcn/ui, prebuilt, no customization

- Use shadcn/ui components **as installed** (`src/components/ui/`). Pull new ones via the shadcn MCP; do not hand-fork component internals.
- Compose at the app level (props, layout, Tailwind tokens) — not by editing primitives.
- Tailwind utility classes via the `cn()` helper (`src/lib/utils.ts`). No ad-hoc global CSS.
- Accessibility is required: labels, focus states, keyboard nav, ARIA on interactive elements (shadcn gives most of this for free — keep it).

## State & data

- **Server state:** TanStack Query only. Centralize fetchers in `src/lib/api.ts`; typed responses matching the API contract.
- **Avoid polling.** The demo polls leads every 30s — replace with WS/SSE live updates (or query invalidation on events) as the backend exposes them.
- **Filter/URL state:** keep table filters/pagination in the URL (searchParams), not a global store.
- Add a global store only if genuinely needed; default to Query + local state.
- Forms: `react-hook-form` + `zod` + shadcn `form`. Validate on the client AND rely on server validation.

## Performance

- Code-split per route (lazy routes); lazy-load the widget.
- Keep the widget bundle minimal (it loads on third-party sites).
- Memoize expensive lists; use shadcn `skeleton` for loading, error boundaries for failures, empty states everywhere.
- Budget: watch bundle size; prefer server-side pagination/filtering over client-side over-fetching.

## Env & secrets

- All backend URLs via `import.meta.env.VITE_*` (e.g. `VITE_API_URL`). Never hardcode a URL.
- **No secrets in the client bundle** — anything in `VITE_*` ships to the browser, so only public config goes there. Real secrets stay backend-side.

## i18n

- RO/EN across the dashboard and widget. Keep user-facing strings in a structured resource (not inline literals) so a language can be added without code surgery. One language per chat session (matches the agent's language stickiness).

## Roles

- Render by role (staff/admin; P2 provider/country_operator). Hide actions the role can't perform — and rely on backend RBAC as the real gate (the UI is not the security boundary). See the RBAC matrix in `docs/specs/SECURITY.md`.

## Quality

- TypeScript strict; no `any` on API boundaries (types mirror the contract).
- `eslint` + `prettier` clean (configs at repo root / frontend).
- Error boundaries at route level; user-visible network errors via shadcn `sonner`/toast, not silent failures.
