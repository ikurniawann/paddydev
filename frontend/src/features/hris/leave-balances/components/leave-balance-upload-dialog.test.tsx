import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { LeaveBalanceImportChange, LeaveBalanceImportPreview } from "../types";
import { LeaveBalanceUploadDialog } from "./leave-balance-upload-dialog";

const api = vi.hoisted(() => ({
  importLeaveBalanceFile: vi.fn(),
  downloadLeaveBalanceErrors: vi.fn(),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast }));
afterEach(cleanup);
beforeEach(() => {
  api.importLeaveBalanceFile.mockReset();
  api.downloadLeaveBalanceErrors.mockReset();
  toast.success.mockReset();
  toast.error.mockReset();
});

const summary = { employees: 2, new: 1, changed: 1, same: 5 };
const change: LeaveBalanceImportChange = {
  nip: "2025001",
  name: "Ana Lestari",
  new: false,
  reason: "Koreksi jatah",
  before: { annual_leave_total: 12, annual_leave_adjustment: 0, carry_over_days: 0, carry_over_expires_on: null },
  after: { annual_leave_total: 12, annual_leave_adjustment: 1.5, carry_over_days: 2, carry_over_expires_on: "2027-03-31" },
};
const preview = (over: Partial<LeaveBalanceImportPreview> = {}) => ({
  data: { summary, errors: [], warnings: [], fingerprint: "fp1", changes: [change], ...over },
});

function setup(onSaved = vi.fn()) {
  const qc = new QueryClient();
  const spy = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <LeaveBalanceUploadDialog open onOpenChange={() => {}} year={2026} branchId="b1" branchName="PPJA" onSaved={onSaved} />
    </QueryClientProvider>
  );
  fireEvent.change(screen.getByLabelText("File saldo cuti (.xlsx)"), { target: { files: [new File(["x"], "saldo.xlsx")] } });
  return { spy, onSaved };
}
const check = () => fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
const saveBtn = () => screen.getByRole("button", { name: "Simpan saldo" }) as HTMLButtonElement;

it("pratinjau menampilkan ringkasan dan perubahan sebelum → sesudah, simpan aktif tanpa error/peringatan", async () => {
  api.importLeaveBalanceFile.mockResolvedValueOnce(preview());
  setup();
  check();
  expect(await screen.findByText(/2 karyawan · 1 baru · 1 berubah · 5 sama/)).toBeTruthy();
  expect(api.importLeaveBalanceFile).toHaveBeenCalledWith(expect.objectContaining({ step: "preview", year: 2026, branch_id: "b1" }));
  const row = screen.getByText("Ana Lestari").closest("tr")!;
  expect(within(row).getByText("2025001")).toBeTruthy();
  expect(within(row).getByText("12 → 12")).toBeTruthy(); // jatah
  expect(within(row).getByText("0 → 1,5")).toBeTruthy(); // penyesuaian
  expect(within(row).getByText("0 → 2")).toBeTruthy(); // sisa bawaan
  expect(within(row).getByText(/—\s*→\s*/)).toBeTruthy(); // kedaluwarsa kosong → tanggal
  expect(within(row).getByText("Koreksi jatah")).toBeTruthy();
  expect(saveBtn().disabled).toBe(false);
});

it("galat menahan simpan dan menawarkan unduh file error", async () => {
  api.importLeaveBalanceFile.mockResolvedValueOnce(
    preview({ errors: [{ line: 7, column: "Jatah", name: "Budi", message: "Jatah harus kelipatan 0,5" }], changes: [] })
  );
  setup();
  check();
  expect(await screen.findByText("Baris 7 · Jatah · Budi")).toBeTruthy();
  expect(screen.getByText("Jatah harus kelipatan 0,5")).toBeTruthy();
  expect(saveBtn().disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Unduh file dengan keterangan error" }));
  await waitFor(() =>
    expect(api.downloadLeaveBalanceErrors).toHaveBeenCalledWith(expect.objectContaining({ year: 2026, branch_id: "b1", file: expect.any(File) }))
  );
});

it("peringatan wajib dicentang, lalu commit memakai fingerprint, menyegarkan daftar, dan menampilkan toast", async () => {
  api.importLeaveBalanceFile
    .mockResolvedValueOnce(preview({ warnings: [{ line: 3, column: "Jatah", name: "Ana", message: "Jatah 30 hari lebih dari 24" }] }))
    .mockResolvedValueOnce({ data: { summary }, message: "Saldo cuti 1 karyawan disimpan" });
  const { spy, onSaved } = setup();
  check();
  await screen.findByText("Jatah 30 hari lebih dari 24");
  expect(saveBtn().disabled).toBe(true);
  fireEvent.click(screen.getByLabelText("Saya sudah memeriksa peringatan"));
  expect(saveBtn().disabled).toBe(false);
  fireEvent.click(saveBtn());
  await waitFor(() =>
    expect(api.importLeaveBalanceFile).toHaveBeenLastCalledWith(
      expect.objectContaining({ step: "commit", fingerprint: "fp1", ack_warnings: true, year: 2026, branch_id: "b1" })
    )
  );
  await waitFor(() => expect(onSaved).toHaveBeenCalled());
  expect(toast.success).toHaveBeenCalledWith("Saldo cuti 1 karyawan disimpan");
  expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "leave-balances"] });
});

it("409 menampilkan pesan server, membuang pratinjau, dan meminta periksa ulang", async () => {
  const stale = Object.assign(new Error("Saldo cuti sudah diubah orang lain sejak preview — unggah ulang file"), { status: 409 });
  api.importLeaveBalanceFile.mockResolvedValueOnce(preview()).mockRejectedValueOnce(stale);
  const { onSaved } = setup();
  check();
  await screen.findByText("Ana Lestari");
  fireEvent.click(saveBtn());
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain("Saldo cuti sudah diubah orang lain sejak preview — unggah ulang file");
  expect(screen.getByRole("alert").textContent).toContain("Periksa file");
  expect(screen.queryByText("Ana Lestari")).toBeNull();
  expect(saveBtn().disabled).toBe(true);
  expect(onSaved).not.toHaveBeenCalled();
  // periksa ulang menghasilkan fingerprint baru
  api.importLeaveBalanceFile.mockResolvedValueOnce(preview({ fingerprint: "fp2" }));
  check();
  await screen.findByText("Ana Lestari");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(saveBtn().disabled).toBe(false);
});

it("400 saat simpan menampilkan pesan server dan mempertahankan pratinjau", async () => {
  api.importLeaveBalanceFile
    .mockResolvedValueOnce(preview())
    .mockRejectedValueOnce(Object.assign(new Error("File masih punya error — perbaiki lalu unggah ulang"), { status: 400 }));
  setup();
  check();
  await screen.findByText("Ana Lestari");
  fireEvent.click(saveBtn());
  expect((await screen.findByRole("alert")).textContent).toContain("File masih punya error — perbaiki lalu unggah ulang");
  expect(screen.getByText("Ana Lestari")).toBeTruthy();
});

it("memberi tahu bila perubahan lebih banyak dari yang ditampilkan", async () => {
  api.importLeaveBalanceFile.mockResolvedValueOnce(
    preview({ summary: { employees: 900, new: 100, changed: 500, same: 300 }, changes: Array.from({ length: 500 }, (_, i) => ({ ...change, nip: `N${i}`, name: `Orang ${i}` })) })
  );
  setup();
  check();
  expect(await screen.findByText(/Menampilkan 500 dari 600 perubahan/)).toBeTruthy();
});
