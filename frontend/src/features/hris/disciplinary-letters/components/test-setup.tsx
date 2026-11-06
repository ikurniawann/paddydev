import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import type { DisciplinaryLetter } from "../types";

export function renderWithClient(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

export const letter = (over: Partial<DisciplinaryLetter> = {}): DisciplinaryLetter => ({
  id: "l1",
  employee_id: "e1",
  employee_name: "Ani Wijaya",
  nip: "1001",
  letter_type: "sp1",
  label: "Surat Peringatan 1",
  issued_date: "2026-11-05",
  notes: "Terlambat 3 kali",
  attachment_path: null,
  created_by_name: "HR Admin",
  created_at: "2026-11-05T03:00:00Z",
  revoked_at: null,
  revoked_by_name: null,
  revoke_reason: null,
  ...over,
});
