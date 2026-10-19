import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ScheduleUploadDialog } from "./schedule-upload-dialog";

const api = vi.hoisted(() => ({
  importScheduleFile: vi.fn(),
  downloadScheduleErrors: vi.fn(),
}));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(cleanup);
beforeEach(() => {
  api.importScheduleFile.mockReset();
  api.downloadScheduleErrors.mockReset();
});

const range = { branch_id: "b1", from: "2026-11-02", to: "2026-11-08" };

it("menahan simpan sampai error nol dan peringatan dicentang", async () => {
  api.importScheduleFile.mockResolvedValueOnce({
    data: {
      summary: { employees: 2, new: 10, changed: 2, same: 2 },
      errors: [{ line: 5, column: "Sel 03/11", name: "Ana", message: "Kode 'SPG 1' tidak ada; maksud Anda SPG1?" }],
      warnings: [], fingerprint: "x", changes: [],
    },
  });
  render(<ScheduleUploadDialog open onOpenChange={() => {}} range={range} branchName="PPJA" onSaved={() => {}} />);
  fireEvent.change(screen.getByLabelText("File jadwal (.xlsx)"), { target: { files: [new File(["x"], "j.xlsx")] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
  expect(await screen.findByText("Baris 5 · Sel 03/11 · Ana")).toBeTruthy();
  expect(screen.getByText("Kode 'SPG 1' tidak ada; maksud Anda SPG1?")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Simpan jadwal" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByRole("button", { name: "Unduh file dengan keterangan error" })).toBeTruthy();
});

it("peringatan wajib dicentang lalu menyimpan dengan fingerprint", async () => {
  api.importScheduleFile
    .mockResolvedValueOnce({ data: { summary: { employees: 1, new: 7, changed: 0, same: 0 }, errors: [],
      warnings: [{ line: 3, column: "Sab 07/11", name: "", message: "Tidak ada yang masuk di PPJA 07/11/2026" }], fingerprint: "fp1", changes: [] } })
    .mockResolvedValueOnce({ data: { summary: { employees: 1, new: 7, changed: 0, same: 0 } }, message: "Jadwal 1 karyawan disimpan" });
  const onSaved = vi.fn();
  render(<ScheduleUploadDialog open onOpenChange={() => {}} range={range} branchName="PPJA" onSaved={onSaved} />);
  fireEvent.change(screen.getByLabelText("File jadwal (.xlsx)"), { target: { files: [new File(["x"], "j.xlsx")] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
  await screen.findByText("Tidak ada yang masuk di PPJA 07/11/2026");
  const save = screen.getByRole("button", { name: "Simpan jadwal" }) as HTMLButtonElement;
  expect(save.disabled).toBe(true);
  fireEvent.click(screen.getByLabelText("Saya sudah memeriksa peringatan"));
  fireEvent.click(save);
  await waitFor(() => expect(api.importScheduleFile).toHaveBeenLastCalledWith(
    expect.objectContaining({ step: "commit", fingerprint: "fp1", ack_warnings: true })));
  expect(onSaved).toHaveBeenCalled();
});

it("meminta konfirmasi sebelum mengganti jadwal yang sudah ada", async () => {
  api.importScheduleFile
    .mockResolvedValueOnce({ data: { summary: { employees: 1, new: 0, changed: 2, same: 5 }, errors: [], warnings: [], fingerprint: "fp2",
      changes: [{ nip: "111", name: "Ana", date: "2026-11-03", before: "SPG1", after: "L" }] } })
    .mockResolvedValueOnce({ data: { summary: { employees: 1, new: 0, changed: 2, same: 5 } }, message: "Jadwal 1 karyawan disimpan" });
  render(<ScheduleUploadDialog open onOpenChange={() => {}} range={range} branchName="PPJA" onSaved={() => {}} />);
  fireEvent.change(screen.getByLabelText("File jadwal (.xlsx)"), { target: { files: [new File(["x"], "j.xlsx")] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
  await screen.findByText(/1 karyawan · 0 baru · 2 berubah · 5 sama/);
  fireEvent.click(screen.getByRole("button", { name: "Simpan jadwal" }));
  expect(await screen.findByText("2 jadwal yang sudah ada akan diganti")).toBeTruthy();
  expect(screen.getByText("Ana · 03/11 · SPG1 → L")).toBeTruthy();
  expect(api.importScheduleFile).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Ya, ganti" }));
  await waitFor(() => expect(api.importScheduleFile).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit", fingerprint: "fp2" })));
});
