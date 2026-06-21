-- 010_lookup_tables.sql
-- Concern: extensible-enum LOOKUP TABLES (invariant #2) + Phase-1 seed values.
-- Decision (spec §2): NO native PG ENUM types. verticals / intents / lead_statuses are
-- reference tables keyed by a stable TEXT `code`. Adding a value = one INSERT, zero DDL.
-- Hot tables (leads.vertical/intent/status) FK these by code -> referential integrity + extensibility.
-- Forward-only/additive: brand new tables; seeds are idempotent via ON CONFLICT DO NOTHING.
-- Rollback note: DROP TABLE lead_statuses, intents, verticals; (only before any FK references exist).

CREATE TABLE IF NOT EXISTS verticals (
  code        TEXT PRIMARY KEY,              -- 'angrosist','palletclearance','skalyou'
  label_ro    TEXT NOT NULL,
  label_en    TEXT NOT NULL,
  sort_order  INT  NOT NULL DEFAULT 100,
  active      BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO verticals(code, label_ro, label_en, sort_order) VALUES
  ('angrosist',       'Angrosist',       'Wholesale buyer',  10),
  ('palletclearance', 'PalletClearance', 'Pallet clearance', 20),
  ('skalyou',         'SkalYou',         'Market entry',      30)   -- [P2] seeded now, harmless
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS intents (
  code        TEXT PRIMARY KEY,              -- 'buy','sell','market_entry'
  label_ro    TEXT NOT NULL,
  label_en    TEXT NOT NULL,
  active      BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO intents(code, label_ro, label_en) VALUES
  ('buy',          'Cumparare',          'Buy'),
  ('sell',         'Vanzare',            'Sell'),
  ('market_entry', 'Intrare pe piata',   'Market entry')  -- [P2]
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS lead_statuses (
  code        TEXT PRIMARY KEY,
  label_ro    TEXT NOT NULL,
  label_en    TEXT NOT NULL,
  sort_order  INT  NOT NULL DEFAULT 100,     -- pipeline ordering in the dashboard
  is_terminal BOOLEAN NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO lead_statuses(code, label_ro, label_en, sort_order, is_terminal) VALUES
  ('new',             'Nou',             'New',             10, false),
  ('qualifying',      'In calificare',   'Qualifying',      20, false),
  ('needs_human',     'Necesita operator','Needs human',    30, false),
  ('qualified',       'Calificat',       'Qualified',       40, false),
  ('offer_requested', 'Oferta ceruta',   'Offer requested', 50, false),
  ('offer_sent',      'Oferta trimisa',  'Offer sent',      60, false),
  ('negotiation',     'Negociere',       'Negotiation',     70, false),
  ('won',             'Castigat',        'Won',             80, true),
  ('lost',            'Pierdut',         'Lost',            90, true)
  -- [P2] additive INSERTs later: diagnosed, matched, routed, delivered, client_accepted
ON CONFLICT (code) DO NOTHING;
