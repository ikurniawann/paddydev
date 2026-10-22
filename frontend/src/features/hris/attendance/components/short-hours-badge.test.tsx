import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RosterEmployee } from "../types";
import { RosterRow } from "./roster-row";
import { ShortHoursBadge } from "./short-hours-badge";

vi.mock("next/image", () => ({ default: ({ alt }: { alt: string }) => <span>{alt}</span> }));
vi.mock("./selfie-photo-dialog", () => ({ SelfiePhotoDialog: () => null }));

afterEach(cleanup);

const emp = (isShort: boolean): RosterEmployee => ({
  employee_id: "e1", full_name: "Crew HQ", nip: null, photo_url: null, department_name: null, job_title: null,
  shift: null, status: "hadir", is_overdue: false, leave: null,
  attendance: {
    id: "a1", clock_in: "2026-10-03T02:00:00Z", clock_out: "2026-10-03T10:59:00Z", work_hours: 7.98, is_late: false,
    late_minutes: 0, clock_in_photo_url: null, clock_out_photo_url: null, shift_id: null, clock_in_geofence: null,
    clock_in_distance_m: null, clock_out_geofence: null, clock_out_distance_m: null, notes: null,
    is_short_hours: isShort, min_work_hours: isShort ? 9 : null,
  },
});

describe("label kurang jam (AB-05)", () => {
  it("badge hanya untuk baris berlabel, dengan keterangan jam minimum", () => {
    render(<ShortHoursBadge isShort minHours="9.00" />);
    expect(screen.getByText("Kurang jam / pulang lebih cepat").getAttribute("title")).toBe(
      "Hadir kurang dari 9 jam (jam minimum toko)"
    );
    cleanup();
    const { container } = render(<ShortHoursBadge isShort={false} minHours="9.00" />);
    expect(container.textContent).toBe("");
  });

  it("keterangan sama untuk jam minimum string (rekap/kalender) dan angka (roster)", () => {
    render(<ShortHoursBadge isShort minHours={9} />);
    expect(screen.getByText("Kurang jam / pulang lebih cepat").getAttribute("title")).toBe(
      "Hadir kurang dari 9 jam (jam minimum toko)"
    );
  });

  it("Monitoring (roster) menampilkan label", () => {
    render(<RosterRow emp={emp(true)} isToday={false} />);
    expect(screen.getByText("Kurang jam / pulang lebih cepat")).toBeTruthy();
    cleanup();
    render(<RosterRow emp={emp(false)} isToday={false} />);
    expect(screen.queryByText("Kurang jam / pulang lebih cepat")).toBeNull();
  });
});
