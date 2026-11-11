-- =============================================================================
-- EPIC-104 R3: label versi bagan unik tanpa membedakan huruf besar/kecil
-- ("Draft 7" dan "draft 7" tidak bisa sama-sama ada). Data saat ini hanya
-- Draft 6. Idempoten. Rollback: DROP INDEX hris.uq_org_chart_versions_label_lower.
-- =============================================================================
CREATE UNIQUE INDEX IF NOT EXISTS uq_org_chart_versions_label_lower
  ON hris.org_chart_versions (lower(label));
