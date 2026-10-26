import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { OvertimeRow } from "../types";
import { OvertimeTable } from "./overtime-table";

const base: OvertimeRow = {
  id: "o1", employee_id: "e1", date: "2026-10-06T00:00:00.000Z", start_time: "17:00:00", end_time: "19:00:00",
  hours: "2.00", source: "employee", status: "pending", reason: "Closing", rejection_reason: null,
  day_type: "regular", employee: { id: "e1", full_name: "Crew Koreksi", nip: "N01" },
};

function table(row: OvertimeRow, onDecide = vi.fn(), canCorrect = false, canDecide = true) {
  return (
    <OvertimeTable rows={[row]} loading={false} decidingId={null} onDecide={onDecide} canCorrect={canCorrect}
      canDecide={canDecide} onCorrect={vi.fn()} />
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

  it("pemegang approve: Tolak memanggil onDecide reject tanpa jenis hari", () => {
    const onDecide = vi.fn();
    render(table(base, onDecide));
    fireEvent.click(screen.getByRole("button", { name: "Tolak" }));
    expect(onDecide).toHaveBeenLastCalledWith("o1", "reject");
  });

  it("pemegang approve: pilihan jenis hari tampil (mengikuti canDecide, bukan role)", () => {
    render(table(base, vi.fn()));
    expect(screen.getByLabelText("Jenis hari")).toBeTruthy();
  });
});

describe("OvertimeTable — istirahat, dokumentasi, koreksi (EPIC-066)", () => {
  it("menampilkan istirahat dan tautan dokumentasi", () => {
    render(table({ ...base, break_minutes: 30, attachments: ["overtime-attachments/e1/1-aaaaaaaaaaaaaaaa.pdf"] }));
    expect(screen.getByText(/istirahat 30 menit/)).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Dokumentasi 1" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Dokumentasi 1" }));
    expect(screen.getByTitle("Dokumentasi 1").getAttribute("src")).toBe(
      "/api/hris/overtime/attachment/overtime-attachments/e1/1-aaaaaaaaaaaaaaaa.pdf");
  });
  it("tombol Koreksi disembunyikan untuk lembur karyawan yang punya atasan langsung", () => {
    render(table({ ...base, employee: { ...base.employee!, reporting_to: "m1", reporting_to_active: true } }, vi.fn(), true));
    expect(screen.queryByRole("button", { name: "Koreksi" })).toBeNull();
  });
  it("tombol Koreksi tampil bila atasan langsung nonaktif (EPIC-063 R9)", () => {
    render(table({ ...base, employee: { ...base.employee!, reporting_to: "m1", reporting_to_active: false } }, vi.fn(), true));
    expect(screen.getByRole("button", { name: "Koreksi" })).toBeTruthy();
  });
  it("tombol Koreksi tetap ada tanpa atasan, atau bila atasan = diri sendiri, atau sumber perusahaan", () => {
    const { unmount } = render(table({ ...base, employee: { ...base.employee!, reporting_to: null } }, vi.fn(), true));
    expect(screen.getByRole("button", { name: "Koreksi" })).toBeTruthy();
    unmount();
    const r2 = render(table({ ...base, employee: { ...base.employee!, reporting_to: "e1" } }, vi.fn(), true));
    expect(screen.getByRole("button", { name: "Koreksi" })).toBeTruthy();
    r2.unmount();
    render(table({ ...base, source: "company", employee: { ...base.employee!, reporting_to: "m1" } }, vi.fn(), true));
    expect(screen.getByRole("button", { name: "Koreksi" })).toBeTruthy();
  });
});


describe("OvertimeTable — tombol keputusan mengikuti akses approve (EPIC-067 R14)", () => {
  it("canDecide=false: Setujui, Tolak, dan pilihan jenis hari tidak dirender", () => {
    render(table(base, vi.fn(), false, false));
    expect(screen.queryByRole("button", { name: "Setujui" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Tolak" })).toBeNull();
    expect(screen.queryByLabelText("Jenis hari")).toBeNull();
  });
  // Batalkan penugasan perusahaan oleh HR butuh approve di Lembur (server: CanDecideOvertime,
  // EPIC-067 R11), jadi ikut canDecide. Koreksi tetap mengikuti canCorrect.
  it("canDecide=false: Batalkan penugasan perusahaan disembunyikan, Koreksi tidak ikut hilang", () => {
    render(table({ ...base, source: "company" }, vi.fn(), true, false));
    expect(screen.queryByRole("button", { name: "Batalkan" })).toBeNull();
    expect(screen.getByText("Menunggu konfirmasi karyawan")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Koreksi" })).toBeTruthy();
  });
  it("canDecide=true: Batalkan penugasan perusahaan memanggil onDecide cancel", () => {
    const onDecide = vi.fn();
    render(table({ ...base, source: "company" }, onDecide, false, true));
    fireEvent.click(screen.getByRole("button", { name: "Batalkan" }));
    expect(onDecide).toHaveBeenCalledWith("o1", "cancel");
  });
});
