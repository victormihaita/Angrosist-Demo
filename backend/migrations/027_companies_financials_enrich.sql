-- 027_companies_financials_enrich.sql
-- Concern: surface the fuller DemoANAF dataset on the company record and its
-- financial-year sidecar (ADDITIVELY; invariants #1, #5 — forward-only/additive,
-- never reshape existing companies/financials data).
--
-- companies: ONRC J-number was already mapped into the domain but had no column;
--   add it plus incorporation date, legal form and the e-Factura flag. These come
--   from /company/:cui (registrationNumber/registrationDate/legalForm/eFacturaRegistered).
-- company_financials: turnover already exists (014). Add net profit + average
--   employees so the dashboard can show a multi-year financial picture.
--
-- Rollback note:
--   ALTER TABLE companies DROP COLUMN registration_number, registration_date, legal_form, e_factura;
--   ALTER TABLE company_financials DROP COLUMN net_profit, employees;

-- 1) companies: richer identity fields (all nullable — foreign / unverified
--    companies and the save_lead recovery path won't have them).
ALTER TABLE companies ADD COLUMN IF NOT EXISTS registration_number TEXT;       -- ONRC J-number (e.g. J40/372/2002); distinct from reg_no/cui
ALTER TABLE companies ADD COLUMN IF NOT EXISTS registration_date   DATE;       -- incorporation date ("when created")
ALTER TABLE companies ADD COLUMN IF NOT EXISTS legal_form          TEXT;       -- SA / SRL / PFA ...
ALTER TABLE companies ADD COLUMN IF NOT EXISTS e_factura           BOOLEAN;    -- registered for e-Factura (eFacturaRegistered)

-- 2) company_financials: per-year net profit + average employee count alongside
--    the existing turnover. Nullable: not every year/indicator is reported.
ALTER TABLE company_financials ADD COLUMN IF NOT EXISTS net_profit NUMERIC(16,2);
ALTER TABLE company_financials ADD COLUMN IF NOT EXISTS employees  INT;
