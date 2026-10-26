import { describe, expect, it } from "vitest";
import { essLeaveBalanceOf, mergeRecentRequests, todayShiftOf, weekScheduleOf, type ScheduleRow } from "./me-profile";

const pagi = { shift_name: "Pagi", start_time: "08:00:00", end_time: "16:00:00", late_tolerance_minutes: 10 };
const none = { shift_name: null, start_time: null, end_time: null, late_tolerance_minutes: null };

// Senin–Jumat Pagi, Sabtu libur, Minggu tanpa pola; berlaku sejak 2026-01-01
const rows: ScheduleRow[] = [
  ...[1, 2, 3, 4, 5].map((day) => ({
    day_of_week: day,
    shift_id: "s-pagi",
    effective_from: "2026-01-01",
    effective_to: null,
    ...pagi,
  })),
  { day_of_week: 6, shift_id: null, effective_from: "2026-01-01", effective_to: null, ...none },
];

describe("todayShiftOf", () => {
  it("hari kerja → detail shift; libur atau tanpa pola → null", () => {
    expect(todayShiftOf(rows, "2026-10-05")).toEqual({
      name: "Pagi",
      start_time: "08:00:00",
      end_time: "16:00:00",
      late_tolerance_minutes: 10,
    });
    expect(todayShiftOf(rows, "2026-10-10")).toBeNull(); // Sabtu libur
    expect(todayShiftOf(rows, "2026-10-11")).toBeNull(); // Minggu tanpa pola
  });
});

describe("weekScheduleOf", () => {
  it("Senin–Minggu dengan status shift/libur/none", () => {
    const week = weekScheduleOf(rows, "2026-10-07"); // Rabu
    expect(week.map((d) => d.date)).toEqual([
      "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08",
      "2026-10-09", "2026-10-10", "2026-10-11",
    ]);
    expect(week.map((d) => d.status)).toEqual([
      "shift", "shift", "shift", "shift", "shift", "libur", "none",
    ]);
    expect(week[2]).toMatchObject({ is_today: true, day_of_week: 3, shift_name: "Pagi" });
    expect(week[5]).toMatchObject({ shift_name: null, start_time: null });
  });
});

describe("mergeRecentRequests", () => {
  it("gabung, urut terbaru, maksimal 6, nominal pinjaman berformat rupiah", () => {
    const leaves = [1, 2, 3].map((i) => ({
      id: `l${i}`, leave_type: "annual", start_date: "2026-10-01", end_date: "2026-10-02",
      status: "pending", created_at: `2026-10-0${i}T08:00:00`,
    }));
    const overtime = [4, 5].map((i) => ({
      id: `o${i}`, date: "2026-10-03", start_time: "17:00:00", end_time: "19:30:00",
      status: "approved", created_at: `2026-10-0${i}T08:00:00`,
    }));
    const loans = [6, 7].map((i) => ({
      id: `p${i}`, loan_type: "kasbon", principal_amount: "1500000", status: "pending",
      created_at: `2026-10-0${i}T08:00:00`,
    }));
    const merged = mergeRecentRequests(leaves, overtime, loans);
    expect(merged.map((r) => r.id)).toEqual(["p7", "p6", "o5", "o4", "l3", "l2"]);
    expect(merged[0].detail).toBe("Rp 1.500.000");
    expect(merged[2]).toMatchObject({ kind: "lembur", label: "Lembur 2026-10-03", detail: "17:00–19:30" });
  });
});

describe("essLeaveBalanceOf", () => {
  const row = {
    year: 2026,
    annual_leave_total: "12.00",
    annual_leave_adjustment: "-1.00",
    annual_leave_used: "3.00",
    annual_leave_remaining: "8.00",
    carry_over_days: "2.00",
    carry_over_used: "0.50",
    carry_over_expires_on: "2026-03-31",
  };

  it("memetakan baris DB ke bentuk saldo Go (angka, bawaan, tersedia)", () => {
    expect(essLeaveBalanceOf(row, "2026-03-10")).toEqual({
      year: 2026,
      annual_leave_total: 12,
      annual_leave_adjustment: -1,
      annual_leave_used: 3,
      annual_leave_remaining: 8,
      carry_over_days: 2,
      carry_over_used: 0.5,
      carry_over_remaining: 1.5,
      carry_over_expires_on: "2026-03-31",
      carry_over_active: true,
      available: 9.5,
    });
  });

  it("sisa bawaan kedaluwarsa atau tanpa tanggal tidak dihitung", () => {
    expect(essLeaveBalanceOf(row, "2026-04-01").available).toBe(8);
    expect(essLeaveBalanceOf({ ...row, carry_over_expires_on: null }, "2026-03-10")).toMatchObject({
      carry_over_active: false,
      available: 8,
    });
  });
});
