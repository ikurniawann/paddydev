import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OvertimeApproveDialog } from "./overtime-approve-dialog";

afterEach(cleanup);

const row = { id: "o1", date: "2026-10-21", start_time: "17:00:00", end_time: "21:00:00", break_minutes: 0,
  attachments: ["overtime-attachments/e1/1-abcdefabcdefabcd.pdf"], employee_name: "Crew A" };

describe("OvertimeApproveDialog (EPIC-066)", () => {
  it("atasan mengubah jam & istirahat lalu menyetujui; jam dibayar dihitung ulang", () => {
    const onApprove = vi.fn();
    render(<OvertimeApproveDialog row={row} open onOpenChange={() => {}} onApprove={onApprove} />);
    expect(screen.getByRole("link", { name: /Dokumentasi 1/ }).getAttribute("href")).toBe(
      "/api/hris/overtime/attachment/overtime-attachments/e1/1-abcdefabcdefabcd.pdf");
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "20:00" } });
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "30" } });
    expect(screen.getByText(/2,5 jam dibayar/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onApprove).toHaveBeenCalledWith({ start_time: "17:00", end_time: "20:00", break_minutes: 30 });
  });
  it("menolak jam dibayar < 1 jam tanpa memanggil onApprove", () => {
    const onApprove = vi.fn();
    render(<OvertimeApproveDialog row={row} open onOpenChange={() => {}} onApprove={onApprove} />);
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "17:30" } });
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onApprove).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toBe("Durasi lembur minimal 1 jam");
  });
});
