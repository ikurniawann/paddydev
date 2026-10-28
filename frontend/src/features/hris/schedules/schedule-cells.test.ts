import { describe, expect, it } from "vitest";
import {
  addDays,
  cellKey,
  copyWeek,
  coverChunks,
  dedupeCells,
  expandPattern,
  fillCells,
  mergeGrids,
  mondayOf,
  rowRange,
  shiftTone,
} from "./schedule-cells";
import type { ScheduleGrid } from "./types";

function days(from: string, n: number): string[] {
  return Array.from({ length: n }, (_, i) => addDays(from, i));
}

function grid(from: string, n: number, rows: Record<string, Record<string, string>>): ScheduleGrid {
  const dates = days(from, n);
  return {
    branch: { id: "b1", name: "PVJ", code: "PVJ" },
    dates: dates.map((date) => ({ date, label: date.slice(5), holiday: null })),
    employees: Object.entries(rows).map(([id, codes]) => ({
      id,
      nip: id,
      name: id,
      position: "",
      cells: dates.map((date) => ({ date, code: codes[date] ?? "", source: codes[date] ? "day" : "none" })),
    })),
    non_shift_count: 0,
    shifts: [],
  };
}

describe("tanggal", () => {
  it("addDays dan mondayOf", () => {
    expect(addDays("2026-11-02", 7)).toBe("2026-11-09");
    expect(addDays("2026-11-02", -1)).toBe("2026-11-01");
    expect(mondayOf("2026-10-29")).toBe("2026-10-26"); // Kamis → Senin
    expect(mondayOf("2026-11-01")).toBe("2026-10-26"); // Minggu → Senin sebelumnya
    expect(mondayOf("2026-11-02")).toBe("2026-11-02");
  });

  it("coverChunks memecah rentang jadi potongan 7–45 hari", () => {
    expect(coverChunks("2026-11-02", "2026-11-03")).toEqual([{ from: "2026-11-02", to: "2026-11-08" }]);
    expect(coverChunks("2026-11-02", "2026-11-29")).toEqual([{ from: "2026-11-02", to: "2026-11-29" }]);
    expect(coverChunks("2026-11-02", "2027-01-02")).toEqual([
      { from: "2026-11-02", to: "2026-12-16" },
      { from: "2026-12-17", to: "2027-01-02" },
    ]);
  });
});

describe("expandPattern", () => {
  const pattern = ["P", "P", "P", "P", "P", "L", "L"]; // Sen..Min

  it("Sen–Jum P, Sab–Min L untuk 2 karyawan × 14 hari = 28 sel", () => {
    const g = grid("2026-11-02", 14, { e1: {}, e2: {} });
    const cells = expandPattern(["e1", "e2"], pattern, "2026-11-02", "2026-11-15", g);
    expect(cells).toHaveLength(28);
    expect(cells.filter((c) => c.code === "P")).toHaveLength(20);
    expect(cells.filter((c) => c.code === "L")).toHaveLength(8);
    expect(cells).toContainEqual({ employee_id: "e1", date: "2026-11-07", code: "L" }); // Sabtu
    expect(cells).toContainEqual({ employee_id: "e2", date: "2026-11-09", code: "P" }); // Senin
  });

  it("melewati kotak '-' (tidak ditempatkan) dan tanggal di luar data grid", () => {
    const g = grid("2026-11-02", 7, { e1: { "2026-11-04": "-" }, e2: {} });
    const cells = expandPattern(["e1", "e2"], pattern, "2026-11-02", "2026-11-10", g);
    expect(cells).toHaveLength(13); // 7 + 7 − 1 "-"; 09–10/11 tidak ada di grid
    expect(cells.find((c) => c.employee_id === "e1" && c.date === "2026-11-04")).toBeUndefined();
  });

  it("pola kosong '' berarti hapus jadwal per tanggal", () => {
    const g = grid("2026-11-02", 7, { e1: {} });
    const cells = expandPattern(["e1"], ["", "P", "P", "P", "P", "L", "L"], "2026-11-02", "2026-11-02", g);
    expect(cells).toEqual([{ employee_id: "e1", date: "2026-11-02", code: "" }]);
  });
});

describe("mergeGrids", () => {
  it("menggabungkan kotak karyawan dari beberapa potongan", () => {
    const a = grid("2026-11-02", 7, { e1: { "2026-11-02": "P" } });
    const b = grid("2026-11-09", 7, { e1: {}, e2: { "2026-11-10": "-" } });
    const merged = mergeGrids([a, b]);
    expect(merged.employees.map((e) => e.id)).toEqual(["e1", "e2"]);
    expect(merged.employees[0].cells).toHaveLength(14);
    expect(merged.employees[1].cells).toHaveLength(7);
  });
});

describe("copyWeek", () => {
  it("menyalin per hari dalam minggu untuk karyawan yang ada di keduanya; lewati '-', kosong, sama, dan tanggal lewat", () => {
    const prev = grid("2026-10-26", 7, {
      e1: { "2026-10-26": "P", "2026-10-27": "S", "2026-10-28": "L", "2026-10-29": "P", "2026-10-31": "P" },
      gone: { "2026-10-26": "P" },
    });
    const cur = grid("2026-11-02", 14, {
      e1: { "2026-11-03": "-", "2026-11-05": "P" },
      fresh: {},
    });
    const cells = copyWeek(prev, cur);
    // Sen P, Rab L; Sel '-' dilewati, Kam sudah P, Jum/Min kosong di minggu lalu, Sab P.
    expect(cells).toEqual([
      { employee_id: "e1", date: "2026-11-02", code: "P" },
      { employee_id: "e1", date: "2026-11-04", code: "L" },
      { employee_id: "e1", date: "2026-11-07", code: "P" },
    ]);
    expect(copyWeek(prev, cur, "2026-11-04")).toEqual([
      { employee_id: "e1", date: "2026-11-04", code: "L" },
      { employee_id: "e1", date: "2026-11-07", code: "P" },
    ]);
  });
});

describe("fillCells dan dedupe", () => {
  it("fillCells mengisi setiap kotak terpilih dengan kode yang sama", () => {
    expect(fillCells([cellKey("e1", "2026-11-02"), cellKey("e2", "2026-11-03")], "S")).toEqual([
      { employee_id: "e1", date: "2026-11-02", code: "S" },
      { employee_id: "e2", date: "2026-11-03", code: "S" },
    ]);
    expect(fillCells([cellKey("e1", "2026-11-02")], "")).toEqual([{ employee_id: "e1", date: "2026-11-02", code: "" }]);
  });

  it("kunci yang sama: edit terakhir menang", () => {
    expect(
      dedupeCells([
        { employee_id: "e1", date: "2026-11-02", code: "P" },
        { employee_id: "e2", date: "2026-11-02", code: "P" },
        { employee_id: "e1", date: "2026-11-02", code: "L" },
      ])
    ).toEqual([
      { employee_id: "e1", date: "2026-11-02", code: "L" },
      { employee_id: "e2", date: "2026-11-02", code: "P" },
    ]);
  });
});

describe("rowRange dan shiftTone", () => {
  it("rowRange mengambil kotak di antara dua tanggal dalam satu baris, urutan bebas", () => {
    const dates = days("2026-11-02", 7);
    expect(rowRange("e1", dates, "2026-11-05", "2026-11-03")).toEqual([
      cellKey("e1", "2026-11-03"),
      cellKey("e1", "2026-11-04"),
      cellKey("e1", "2026-11-05"),
    ]);
  });

  it("warna per kode tetap (deterministik)", () => {
    expect(shiftTone("P")).toBe(shiftTone("P"));
    expect(shiftTone("L")).toContain("gray");
    expect(shiftTone("")).toContain("amber");
  });
});
