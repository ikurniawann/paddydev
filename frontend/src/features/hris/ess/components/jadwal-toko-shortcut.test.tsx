import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ScheduleAccess } from "@/features/hris/schedule-access/types";
import { JadwalTokoShortcut } from "./jadwal-toko-shortcut";

let access: ScheduleAccess | undefined;
vi.mock("@/features/hris/schedule-access/queries", () => ({ useScheduleAccess: () => ({ data: access }) }));

const store = (id: string) => ({ id, code: id.toUpperCase(), name: `Toko ${id}` });

afterEach(() => {
  cleanup();
  access = undefined;
});

describe("JadwalTokoShortcut (EPIC-073 R4)", () => {
  it("pengatur: kartu ke Jadwal Toko menyebut jumlah tokonya", () => {
    access = { is_delegate: true, can_place: true, branches: [store("a"), store("b")] };
    render(<JadwalTokoShortcut />);
    const link = screen.getByRole("link", { name: /Jadwal Toko/ });
    expect(link.getAttribute("href")).toBe("/dashboard/me/jadwal-toko");
    expect(screen.getByText(/2 toko/)).toBeTruthy();
    expect(screen.getByText(/penempatan/)).toBeTruthy();
  });

  it("HR Jadwal Kerja: kartu mode HR", () => {
    access = { is_delegate: false, can_place: false, branches: [], hr: true, hr_can_place: false };
    render(<JadwalTokoShortcut />);
    expect(screen.getByRole("link", { name: /Jadwal Toko/ })).toBeTruthy();
    expect(screen.getByText(/semua toko/)).toBeTruthy();
  });

  it("karyawan biasa atau akses belum dimuat: tidak ada kartu", () => {
    access = { is_delegate: false, can_place: false, branches: [], hr: false, hr_can_place: false };
    const { container, rerender } = render(<JadwalTokoShortcut />);
    expect(container.innerHTML).toBe("");
    access = undefined;
    rerender(<JadwalTokoShortcut />);
    expect(container.innerHTML).toBe("");
  });
});
