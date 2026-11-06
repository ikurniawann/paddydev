import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ResignDialog } from "./resign-dialog";

const api = vi.hoisted(() => ({ resignEmployee: vi.fn(async () => ({ data: {}, message: "Karyawan diresign" })) }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(cleanup);

it("wajib tanggal & jenis, lalu mengirim resign", async () => {
  const onDone = vi.fn();
  render(<ResignDialog open employeeId="e1" employeeName="Rina" joinDate="2025-01-10" today="2026-10-21" onOpenChange={() => {}} onDone={onDone} />);
  expect(screen.getByText(/Akun login Rina akan diblokir/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Alasan"), { target: { value: "Pindah kota" } });
  fireEvent.click(screen.getByRole("button", { name: "Resign" }));
  await waitFor(() =>
    expect(api.resignEmployee).toHaveBeenCalledWith("e1", { effective_date: "2026-10-21", resign_type: "resign", reason: "Pindah kota" })
  );
  expect(onDone).toHaveBeenCalled();
});

it("tanggal mendatang menjadi resign terjadwal (EPIC-095)", async () => {
  const onDone = vi.fn();
  api.resignEmployee.mockResolvedValueOnce({ data: { scheduled: true }, message: "Resign dijadwalkan efektif 20-11-2026" });
  render(<ResignDialog open employeeId="e1" employeeName="Rina" joinDate="2025-01-10" today="2026-11-06" onOpenChange={() => {}} onDone={onDone} />);
  fireEvent.change(screen.getByDisplayValue("2026-11-06"), { target: { value: "2026-11-20" } });
  expect(
    screen.getByText("Karyawan tetap aktif sampai 20-11-2026. Akun dinonaktifkan dan login diblokir otomatis pada 21-11-2026 00.00 WIB.")
  ).toBeTruthy();
  expect(screen.queryByText(/Akun login Rina akan diblokir/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Jadwalkan resign" }));
  await waitFor(() =>
    expect(api.resignEmployee).toHaveBeenLastCalledWith("e1", { effective_date: "2026-11-20", resign_type: "resign", reason: null })
  );
  expect(onDone).toHaveBeenCalled();
});

it("lebih dari 180 hari ditolak", () => {
  render(<ResignDialog open employeeId="e1" employeeName="Rina" joinDate="2025-01-10" today="2026-11-06" onOpenChange={() => {}} onDone={() => {}} />);
  fireEvent.change(screen.getByDisplayValue("2026-11-06"), { target: { value: "2027-05-06" } });
  expect(screen.getByText("Tanggal efektif paling jauh 180 hari dari hari ini")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Jadwalkan resign" }) as HTMLButtonElement).disabled).toBe(true);
});
