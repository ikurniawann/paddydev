import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeamLateness } from "../types";
import { TeamLatenessCard } from "./team-lateness-card";
import { lateDay, level, member, teamEnvelope } from "./test-fixtures";

let data: TeamLateness | undefined;
let other: { data?: TeamLateness; isError?: boolean } | undefined;
vi.mock("../queries", () => ({
  useTeamLateness: (back: number) => {
    const base = { data, isError: false, isPlaceholderData: false, refetch: vi.fn() };
    return back > 0 && other ? { ...base, ...other } : base;
  },
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children, onValueChange }: { children: React.ReactNode; onValueChange: (v: string) => void }) => (
    <div>
      <button type="button" onClick={() => onValueChange("1")}>
        pilih September
      </button>
      {children}
    </div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

afterEach(() => {
  cleanup();
  data = undefined;
  other = undefined;
});

const days = (n: number) => Array.from({ length: n }, (_, i) => lateDay(`2026-10-${String(i + 1).padStart(2, "0")}`));

function team(): TeamLateness {
  return {
    ...teamEnvelope,
    summary: { none: 1, teguran: 1, sp1: 0, sp2: 0, sp3: 1 },
    employees: [
      member({ employee_id: "s", full_name: "Staf S", direct: false, reporting_to_name: "Koordinator K", count: 7, level: level("sp3"), days: days(7) }),
      member({ employee_id: "k", full_name: "Koordinator K", count: 1, level: level("teguran"), days: days(1) }),
      member({ employee_id: "q", full_name: "Anggota Tepat" }),
    ],
  };
}

describe("TeamLatenessCard (EPIC-077 R10)", () => {
  it("default: anggota yang terlambat, anggota tidak langsung menyebut atasannya", () => {
    data = team();
    render(<TeamLatenessCard />);
    expect(screen.getByText("Staf S")).toBeTruthy();
    expect(screen.getByText("via Koordinator K")).toBeTruthy();
    expect(screen.getByText("7x")).toBeTruthy();
    expect(screen.queryByText("Anggota Tepat")).toBeNull();
    expect(screen.getByText(/3 anggota/)).toBeTruthy();
  });

  it("chip SP 3 memfilter; chip Tepat waktu menampilkan yang tepat waktu", () => {
    data = team();
    render(<TeamLatenessCard />);
    fireEvent.click(screen.getByRole("button", { name: /SP 3 · 1/ }));
    expect(screen.getByText("Staf S")).toBeTruthy();
    expect(screen.queryByText("Koordinator K", { selector: "span.font-semibold" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Tepat waktu · 1/ }));
    expect(screen.getByText("Anggota Tepat")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /SP 1 · 0/ }));
    expect(screen.getByText("Tidak ada anggota di tingkat ini.")).toBeTruthy();
  });

  it("membuka hari terlambat anggota", () => {
    data = team();
    render(<TeamLatenessCard />);
    const row = screen.getByRole("button", { name: /Staf S.*7x.*SP 3/ });
    expect(screen.queryByText(/Masuk/)).toBeNull();
    fireEvent.click(row);
    expect(row.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getAllByText(/Masuk/)).toHaveLength(7);
  });

  it("periode lain gagal dimuat: kartu tetap ada dengan pesan galat", () => {
    data = team();
    other = { data: undefined, isError: true };
    render(<TeamLatenessCard />);
    fireEvent.click(screen.getByRole("button", { name: "pilih September" }));
    expect(screen.getByText("Keterlambatan tim gagal dimuat.")).toBeTruthy();
    expect(screen.getByText("Keterlambatan Tim")).toBeTruthy();
  });

  it("semua tepat waktu", () => {
    data = { ...team(), summary: { none: 1, teguran: 0, sp1: 0, sp2: 0, sp3: 0 }, employees: [member({ employee_id: "q", full_name: "Anggota Tepat" })] };
    render(<TeamLatenessCard />);
    expect(screen.getByText("Semua anggota tepat waktu di periode ini.")).toBeTruthy();
  });

  it("tanpa tim atau belum dimuat: tidak dirender", () => {
    data = { ...team(), summary: { none: 0, teguran: 0, sp1: 0, sp2: 0, sp3: 0 }, employees: [] };
    const { container, rerender } = render(<TeamLatenessCard />);
    expect(container.innerHTML).toBe("");
    data = undefined;
    rerender(<TeamLatenessCard />);
    expect(container.innerHTML).toBe("");
  });
});
