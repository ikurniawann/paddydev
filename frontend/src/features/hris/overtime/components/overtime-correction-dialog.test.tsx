import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { OvertimeRow } from "../types";
import { OvertimeCorrectionDialog } from "./overtime-correction-dialog";

const correct = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();

vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("../mutations", () => ({ useCorrectOvertime: () => ({ mutate: correct, isPending: false }) }));

const row: OvertimeRow = {
  id: "o1", employee_id: "e1", date: "2026-10-06T00:00:00.000Z", start_time: "17:00:00", end_time: "19:00:00",
  hours: "2.00", source: "employee", status: "approved", reason: "Closing", rejection_reason: null,
  day_type: "regular", employee: { id: "e1", full_name: "Crew Koreksi", nip: "N01" },
};

afterEach(() => {
  cleanup();
  correct.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
});

describe("OvertimeCorrectionDialog — AP-04", () => {
  it("alasan koreksi wajib minimal 5 karakter", () => {
    render(<OvertimeCorrectionDialog row={row} onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Alasan koreksi"), { target: { value: " abc " } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan koreksi" }));
    expect(toastError).toHaveBeenCalledWith("Alasan koreksi minimal 5 karakter");
    expect(correct).not.toHaveBeenCalled();
  });

  it("mengirim tanggal, jam, jenis, dan alasan lalu menutup", () => {
    const onClose = vi.fn();
    correct.mockImplementation((_v, opts) => opts.onSuccess({ message: "Lembur dikoreksi" }));
    render(<OvertimeCorrectionDialog row={row} onClose={onClose} />);
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-06");
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "20:30" } });
    fireEvent.change(screen.getByLabelText("Jenis lembur"), { target: { value: "rest_day" } });
    fireEvent.change(screen.getByLabelText("Alasan koreksi"), { target: { value: " Sesuai absensi " } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan koreksi" }));
    expect(correct).toHaveBeenCalledWith(
      { id: "o1", date: "2026-10-06", start_time: "17:00", end_time: "20:30", day_type: "rest_day", reason: "Sesuai absensi" },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Lembur dikoreksi");
    expect(onClose).toHaveBeenCalled();
  });

  it("istirahat terisi dari baris dan dikirim hanya bila diubah (EPIC-066)", () => {
    render(<OvertimeCorrectionDialog row={{ ...row, break_minutes: 30 }} onClose={vi.fn()} />);
    const brk = screen.getByLabelText("Istirahat (menit)") as HTMLInputElement;
    expect(brk.value).toBe("30");
    fireEvent.change(brk, { target: { value: "60" } });
    fireEvent.change(screen.getByLabelText("Alasan koreksi"), { target: { value: "Sesuai absensi" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan koreksi" }));
    expect(correct).toHaveBeenCalledWith(expect.objectContaining({ id: "o1", break_minutes: 60 }), expect.anything());
  });

  it("menolak istirahat di luar 0–720 menit", () => {
    render(<OvertimeCorrectionDialog row={row} onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "721" } });
    fireEvent.change(screen.getByLabelText("Alasan koreksi"), { target: { value: "Sesuai absensi" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan koreksi" }));
    expect(toastError).toHaveBeenCalledWith("Istirahat 0–720 menit");
    expect(correct).not.toHaveBeenCalled();
  });
});
