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
