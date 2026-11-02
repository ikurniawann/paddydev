import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { HolidayRow } from "../types";
import { HolidayBadges } from "./holiday-badges";
import { HolidayFormDialog } from "./holiday-form-dialog";

const mutate = vi.fn();
vi.mock("../mutations", () => ({
  useSaveHoliday: () => ({ mutate, isPending: false }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const SWITCH_LABEL = "Ditetapkan libur oleh perusahaan";

const cutiBersama: HolidayRow = {
  id: "h1",
  holiday_date: "2026-12-24",
  name: "Cuti Bersama Natal",
  type: "cuti_bersama",
  deducts_leave: true,
  is_day_off: false,
  status: "aktif",
  source: "manual",
  note: null,
};

afterEach(() => {
  cleanup();
  mutate.mockReset();
});

function open(holiday: HolidayRow | null) {
  render(<HolidayFormDialog open holiday={holiday} year={2026} onOpenChange={() => {}} onSaved={() => {}} />);
}

describe("HolidayFormDialog: ditetapkan libur", () => {
  it("menyembunyikan switch untuk libur nasional", () => {
    open(null);
    expect(screen.queryByRole("switch", { name: SWITCH_LABEL })).toBeNull();
  });

  it("menampilkan switch untuk cuti bersama dan mengirim is_day_off true", () => {
    open(cutiBersama);
    const toggle = screen.getByRole("switch", { name: SWITCH_LABEL });
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(mutate).toHaveBeenCalledWith(
      { payload: expect.objectContaining({ type: "cuti_bersama", is_day_off: true }), id: "h1" },
      expect.anything()
    );
  });

  it("tidak mengirim is_day_off true untuk tipe selain cuti bersama", () => {
    open({ ...cutiBersama, type: "nasional", deducts_leave: false });
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(mutate.mock.calls[0][0].payload.is_day_off).toBe(false);
  });
});

describe("HolidayBadges: hari kerja atau libur", () => {
  it("nasional dan perusahaan selalu libur HQ/Operations", () => {
    render(<HolidayBadges type="nasional" deductsLeave={false} isDayOff={false} status="aktif" />);
    render(<HolidayBadges type="perusahaan" deductsLeave={false} isDayOff={false} status="aktif" />);
    expect(screen.getAllByText("Libur HQ/Operations")).toHaveLength(2);
  });

  it("cuti bersama: libur bila ditetapkan, selain itu hari kerja", () => {
    const { rerender } = render(
      <HolidayBadges type="cuti_bersama" deductsLeave isDayOff={false} status="aktif" />
    );
    expect(screen.getByText("Hari kerja")).toBeTruthy();
    expect(screen.queryByText("Libur HQ/Operations")).toBeNull();
    rerender(<HolidayBadges type="cuti_bersama" deductsLeave isDayOff status="aktif" />);
    expect(screen.getByText("Libur HQ/Operations")).toBeTruthy();
  });
});
