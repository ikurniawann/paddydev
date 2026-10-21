import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { OvertimeRow } from "../types";
import { OvertimeTable } from "./overtime-table";

const base: OvertimeRow = {
  id: "o1", employee_id: "e1", date: "2026-10-06T00:00:00.000Z", start_time: "17:00:00", end_time: "19:00:00",
  hours: "2.00", source: "employee", status: "pending", reason: "Closing", rejection_reason: null,
  day_type: "regular", employee: { id: "e1", full_name: "Crew Koreksi", nip: "N01" },
};

function table(row: OvertimeRow, onDecide = vi.fn(), isHrd = true) {
  return (
    <OvertimeTable rows={[row]} loading={false} decidingId={null} onDecide={onDecide} canCorrect={false}
      isHrd={isHrd} onCorrect={vi.fn()} />
  );
}

afterEach(cleanup);

describe("OvertimeTable — jenis hari mengikuti nilai server", () => {
  it("setelah koreksi (refetch) select menampilkan jenis baru dan Setujui tidak menimpanya dengan nilai lama", () => {
    const onDecide = vi.fn();
    const { rerender } = render(table(base, onDecide));
    expect((screen.getByLabelText("Jenis hari") as HTMLSelectElement).value).toBe("regular");

    rerender(table({ ...base, day_type: "rest_day", corrected_at: "2026-10-21T01:00:00Z" }, onDecide));
    const select = screen.getByLabelText("Jenis hari") as HTMLSelectElement;
    expect(select.value).toBe("rest_day");
    expect(screen.getAllByText("Weekend / Libur").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onDecide).toHaveBeenCalledWith("o1", "approve", undefined);
  });

  it("mengirim jenis hanya bila HR mengubahnya", () => {
    const onDecide = vi.fn();
    render(table(base, onDecide));
    fireEvent.change(screen.getByLabelText("Jenis hari"), { target: { value: "rest_day" } });
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onDecide).toHaveBeenCalledWith("o1", "approve", "rest_day");
  });

  it("bukan HRD: tanpa pilihan jenis hari; Setujui tidak mengirim jenis (server mengabaikannya)", () => {
    const onDecide = vi.fn();
    render(table({ ...base, day_type: "rest_day" }, onDecide, false));
    expect(screen.queryByLabelText("Jenis hari")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onDecide).toHaveBeenCalledWith("o1", "approve", undefined);
    fireEvent.click(screen.getByRole("button", { name: "Tolak" }));
    expect(onDecide).toHaveBeenLastCalledWith("o1", "reject");
  });

  it("HRD: pilihan jenis hari tampil", () => {
    render(table(base, vi.fn(), true));
    expect(screen.getByLabelText("Jenis hari")).toBeTruthy();
  });
});
