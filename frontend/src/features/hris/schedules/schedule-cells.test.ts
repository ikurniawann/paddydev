import { describe, expect, it } from "vitest";
import { addDays } from "./schedule-range";
import {
  cellKey,
  copyWeek,
  coverChunks,
  dedupeCells,
  expandPattern,
  fillCells,
  mergeGrids,
  rowRange,
  shiftTone,
  canFollow,
  describeShiftCode,
  joinShiftCodes,
  splitShiftCode,
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

describe("potongan grid", () => {
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

describe("kode dua shift (EPIC-080)", () => {
  const pagi = { code: "PAGI4", name: "Pagi 4", start_time: "08:00:00", end_time: "12:00:00" };
  const sore = { code: "SORE4", name: "Sore 4", start_time: "17:00:00", end_time: "21:00:00" };
  const malam = { code: "NS", name: "Malam", start_time: "22:00:00", end_time: "06:00:00" };
  const byCode = new Map([pagi, sore, malam].map((s) => [s.code, s]));

  it("gabung dan pisah kode", () => {
    expect(joinShiftCodes("PAGI4", "SORE4")).toBe("PAGI4+SORE4");
    expect(joinShiftCodes("PAGI4", "")).toBe("PAGI4");
    expect(joinShiftCodes("PAGI4", null)).toBe("PAGI4");
    expect(splitShiftCode("PAGI4+SORE4")).toEqual(["PAGI4", "SORE4"]);
    expect(splitShiftCode("L")).toEqual(["L"]);
  });

  it("keterangan dua shift memakai format spesifikasi", () => {
    expect(describeShiftCode("PAGI4+SORE4", byCode)).toBe("PAGI4 + SORE4 · 08.00–12.00, 17.00–21.00");
    expect(describeShiftCode("PAGI4", byCode)).toBe("Pagi 4 08:00–12:00");
    expect(describeShiftCode("PAGI4+XX", byCode)).toBeNull();
  });

  it("shift kedua harus mulai setelah shift pertama selesai", () => {
    expect(canFollow(pagi, sore)).toBe(true);
    expect(canFollow(sore, pagi)).toBe(false);
    expect(canFollow(pagi, { start_time: "12:00:00" })).toBe(true);
    expect(canFollow(malam, pagi)).toBe(false);
  });

  it("pola dan isi kotak membawa kode gabungan apa adanya", () => {
    const g = grid("2026-11-02", 7, { e1: {} });
    const out = expandPattern(["e1"], ["PAGI4+SORE4", "L", "L", "L", "L", "L", "L"], "2026-11-02", "2026-11-02", g);
    expect(out).toEqual([{ employee_id: "e1", date: "2026-11-02", code: "PAGI4+SORE4" }]);
    expect(fillCells([cellKey("e1", "2026-11-02")], "PAGI4+SORE4")).toEqual([{ employee_id: "e1", date: "2026-11-02", code: "PAGI4+SORE4" }]);
  });
});
