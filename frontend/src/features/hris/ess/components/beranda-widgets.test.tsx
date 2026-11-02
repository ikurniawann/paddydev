import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { EssBeranda } from "../types";
import { WeekScheduleCard } from "./beranda-widgets";

vi.mock("next/link", () => ({ default: ({ children }: { children: React.ReactNode }) => <a>{children}</a> }));
afterEach(cleanup);

const day = (over: Partial<EssBeranda["week_schedule"][number]>): EssBeranda["week_schedule"][number] => ({
  date: "2026-11-02", day_of_week: 1, is_today: false, status: "shift", shift_name: "Pagi", start_time: "08:00:00", end_time: "16:00:00", ...over,
});

it("jadwal minggu: satu shift tetap menampilkan jam mulai; dua shift menampilkan jam mulai keduanya (EPIC-080)", () => {
  render(
    <WeekScheduleCard
      days={[
        day({}),
        day({
          date: "2026-11-03", day_of_week: 2, shift_name: "PAGI4 + SORE4", end_time: "21:00:00",
          time_label: "08.00–12.00, 17.00–21.00",
        }),
      ]}
    />
  );
  // Satu format di kedua sel: "08:00" (sel dua shift menyumbang satu 08:00 juga).
  expect(screen.getAllByText("08:00")).toHaveLength(2);
  const two = screen.getByTitle("PAGI4 + SORE4 · 08.00–12.00, 17.00–21.00");
  expect(two.textContent).toBe("08:0017:00");
});
