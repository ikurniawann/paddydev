-- Approval kasbon dua tahap (EPIC-060, MOM AP-05): approver manager disimpan
-- saat pengajuan; status antara 'manager_approved' sebelum HR Penggajian.
ALTER TABLE hris.loans
  ADD COLUMN IF NOT EXISTS manager_approver_id uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_by uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_loans_manager_approver ON hris.loans (manager_approver_id) WHERE status = 'pending';
