-- 012_categories.sql
-- Concern: shared hierarchical taxonomy (§3.5). Matching enabler; referenced by
-- sourcing_requests / listings / buyer_profiles.
-- parent_id ON DELETE RESTRICT: protect the tree (invariant: taxonomy is RESTRICT) -- §4.1.
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE categories;

CREATE TABLE IF NOT EXISTS categories (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id  UUID REFERENCES categories(id) ON DELETE RESTRICT,  -- RESTRICT: protect taxonomy tree
  name       TEXT NOT NULL,
  code       TEXT NOT NULL UNIQUE,                  -- stable slug used by agent + matching
  sort_order INT  NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query: list children of a category / walk the tree by parent.
CREATE INDEX IF NOT EXISTS categories_parent_idx ON categories (parent_id);
