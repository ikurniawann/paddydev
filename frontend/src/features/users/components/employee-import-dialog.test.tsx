import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { EmployeeImportDialog } from "./employee-import-dialog";

const api = vi.hoisted(() => ({ importEmployeesFile: vi.fn(), downloadEmployeeImportErrors: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(cleanup);
beforeEach(() => api.importEmployeesFile.mockReset());

function pick(mode: "Tambah" | "Timpa") {
  render(<EmployeeImportDialog open onOpenChange={() => {}} onDone={() => {}} />);
  fireEvent.click(screen.getByRole("radio", { name: new RegExp(mode) }));
  fireEvent.change(screen.getByLabelText("File data karyawan"), { target: { files: [new File(["x"], "data.xlsx")] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
}

it("error menahan simpan dan bisa unduh file keterangan error", async () => {
  api.importEmployeesFile.mockResolvedValueOnce({
    data: { summary: { create: 1, update: 0, skip: 0 }, errors: [{ line: 4, column: "Email", name: "", message: "wajib diisi untuk karyawan baru" }],
      warnings: [], rows: [], changes: [] },
  });
  pick("Tambah");
  expect(await screen.findByText("Baris 4, kolom Email: wajib diisi untuk karyawan baru")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Simpan" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByRole("button", { name: "Unduh file dengan keterangan error" })).toBeTruthy();
});

it("mode Timpa minta konfirmasi dengan daftar Sebelum → Sesudah", async () => {
  api.importEmployeesFile
    .mockResolvedValueOnce({
      data: { summary: { create: 0, update: 2, skip: 5 }, errors: [], warnings: [], rows: [],
        changes: [{ line: 2, nip: "111", name: "Feti", field: "Tipe Jadwal", before: "Shift", after: "Non-Shift" }] },
    })
    .mockResolvedValueOnce({ data: { summary: { create: 0, update: 2, skip: 5 } }, message: "0 karyawan ditambahkan, 2 diperbarui" });
  pick("Timpa");
  await screen.findByText(/0 baru · 2 diperbarui · 5 tidak berubah/);
  fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
  expect(await screen.findByText("2 karyawan akan diperbarui")).toBeTruthy();
  expect(screen.getAllByText("Feti · Tipe Jadwal · Shift → Non-Shift").length).toBeGreaterThan(0);
  expect(api.importEmployeesFile).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Ya, timpa data" }));
  await waitFor(() => expect(api.importEmployeesFile).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit", mode: "replace" })));
});

it("email karyawan resign wajib dikonfirmasi lepas", async () => {
  api.importEmployeesFile
    .mockResolvedValueOnce({
      data: { summary: { create: 1, update: 0, skip: 0 }, errors: [], rows: [], changes: [],
        warnings: [{ line: 3, column: "Email", name: "", message: "lama@x.id milik Lama (NIP 1, resign) — akan dilepas bila dikonfirmasi" }] },
    })
    .mockResolvedValueOnce({ data: { summary: { create: 1, update: 0, skip: 0 } }, message: "1 karyawan ditambahkan, 0 diperbarui" });
  pick("Tambah");
  await screen.findByText(/akan dilepas bila dikonfirmasi/);
  const save = screen.getByRole("button", { name: "Simpan" }) as HTMLButtonElement;
  expect(save.disabled).toBe(true);
  fireEvent.click(screen.getByLabelText(/Lepas email dari data karyawan resign/));
  fireEvent.click(save);
  await waitFor(() => expect(api.importEmployeesFile).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit", release_emails: true })));
});
