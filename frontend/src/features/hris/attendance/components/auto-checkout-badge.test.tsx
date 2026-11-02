import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RosterEmployee } from "../types";
import { AutoCheckoutBadge } from "./auto-checkout-badge";
import { RosterRow } from "./roster-row";

vi.mock("next/image", () => ({ default: ({ alt }: { alt: string }) => <span>{alt}</span> }));
vi.mock("./selfie-photo-dialog", () => ({ SelfiePhotoDialog: () => null }));

afterEach(cleanup);

const emp = (auto: boolean): RosterEmployee => ({
  employee_id: "e1", full_name: "Crew Toko", nip: null, photo_url: null, department_name: null, job_title: null,
  shift: null, status: "hadir", is_overdue: false, leave: null,
  attendance: {
    id: "a1", clock_in: "2026-10-03T01:00:00Z", clock_out: "2026-10-03T09:00:00Z", work_hours: 7, is_late: false,
    late_minutes: 0, clock_in_photo_url: null, clock_out_photo_url: null, shift_id: null, clock_in_geofence: null,
    clock_in_distance_m: null, clock_out_geofence: null, clock_out_distance_m: null, notes: null, clock_out_auto: auto,
  },
});

describe("penanda Checkout otomatis (EPIC-080 R4)", () => {
  it("badge hanya untuk baris yang ditutup sistem", () => {
    render(<AutoCheckoutBadge auto />);
    expect(screen.getByText("Checkout otomatis")).toBeTruthy();
    cleanup();
    expect(render(<AutoCheckoutBadge auto={false} />).container.textContent).toBe("");
    expect(render(<AutoCheckoutBadge />).container.textContent).toBe("");
  });

  it("Monitoring (roster) menampilkan penanda", () => {
    render(<RosterRow emp={emp(true)} isToday={false} />);
    expect(screen.getByText("Checkout otomatis")).toBeTruthy();
    cleanup();
    render(<RosterRow emp={emp(false)} isToday={false} />);
    expect(screen.queryByText("Checkout otomatis")).toBeNull();
  });
});
