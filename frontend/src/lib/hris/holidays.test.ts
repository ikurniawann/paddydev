import { describe, expect, test } from "vitest";
import {
  countLeaveDays,
  describeLeaveDays,
  eachDateIso,
  holidaysOn,
  indexHolidays,
  isHoliday,
  isHolidayDayOff,
  isWeekend,
  type HolidayRow,
} from "./holidays";

/** Potongan nyata dari seed 2026 — Agustus & klaster Idul Fitri. */
const ROWS: HolidayRow[] = [
  { holiday_date: "2026-03-20", name: "Cuti Bersama Idul Fitri", type: "cuti_bersama", deducts_leave: true },
  { holiday_date: "2026-03-21", name: "Hari Raya Idul Fitri 1447 H", type: "nasional", deducts_leave: false },
  { holiday_date: "2026-03-23", name: "Cuti Bersama Idul Fitri (2)", type: "cuti_bersama", deducts_leave: true },
  { holiday_date: "2026-03-24", name: "Cuti Bersama Idul Fitri (3)", type: "cuti_bersama", deducts_leave: true },
  { holiday_date: "2026-08-17", name: "Hari Kemerdekaan Republik Indonesia", type: "nasional", deducts_leave: false },
];

const index = indexHolidays(ROWS);

describe("isWeekend", () => {
  test("Sabtu dan Minggu", () => {
    expect(isWeekend("2026-08-15")).toBe(true); // Sabtu
    expect(isWeekend("2026-08-16")).toBe(true); // Minggu
  });

  test("Senin sampai Jumat bukan akhir pekan", () => {
    expect(isWeekend("2026-08-17")).toBe(false); // Senin
    expect(isWeekend("2026-08-21")).toBe(false); // Jumat
  });
});

describe("eachDateIso", () => {
  test("rentang inklusif di kedua ujung", () => {
    expect(eachDateIso("2026-08-17", "2026-08-19")).toEqual([
      "2026-08-17",
      "2026-08-18",
      "2026-08-19",
    ]);
  });

  test("rentang satu hari", () => {
    expect(eachDateIso("2026-08-17", "2026-08-17")).toEqual(["2026-08-17"]);
  });

  test("melintasi batas bulan", () => {
    expect(eachDateIso("2026-07-30", "2026-08-02")).toEqual([
      "2026-07-30",
      "2026-07-31",
      "2026-08-01",
      "2026-08-02",
    ]);
  });

  test("akhir sebelum awal menghasilkan rentang kosong", () => {
    expect(eachDateIso("2026-08-19", "2026-08-17")).toEqual([]);
  });

  test("format tanggal tidak valid ditolak", () => {
    expect(() => eachDateIso("17-08-2026", "2026-08-19")).toThrow();
  });
});

describe("holidaysOn / isHoliday", () => {
  test("mengembalikan baris libur pada tanggalnya", () => {
    expect(holidaysOn(index, "2026-08-17").map((h) => h.name)).toEqual([
      "Hari Kemerdekaan Republik Indonesia",
    ]);
    expect(isHoliday(index, "2026-08-17")).toBe(true);
  });

  test("hari kerja biasa bukan libur", () => {
    expect(holidaysOn(index, "2026-08-18")).toEqual([]);
    expect(isHoliday(index, "2026-08-18")).toBe(false);
  });

  test("index kosong aman", () => {
    expect(isHoliday(indexHolidays([]), "2026-08-17")).toBe(false);
  });
});

describe("countLeaveDays", () => {
  test("rentang tanpa libur = jumlah hari kerja", () => {
    // Sen 10 – Jum 14 Agustus 2026
    expect(countLeaveDays("2026-08-10", "2026-08-14", index)).toBe(5);
  });

  test("akhir pekan tidak memotong jatah", () => {
    // Jum 14 – Sen 17 Agustus: Sabtu & Minggu dilewati, 17 Agustus libur nasional
    expect(countLeaveDays("2026-08-14", "2026-08-17", index)).toBe(1);
  });

  test("libur nasional di tengah rentang tidak memotong jatah", () => {
    // Sen 17 (merdeka) – Rab 19 Agustus → hanya 18 & 19 yang terpotong
    expect(countLeaveDays("2026-08-17", "2026-08-19", index)).toBe(2);
  });

  test("cuti bersama TETAP memotong jatah", () => {
    // Jum 20 Maret 2026 = Cuti Bersama Idul Fitri
    expect(countLeaveDays("2026-03-20", "2026-03-20", index)).toBe(1);
  });

  test("klaster Idul Fitri: hanya cuti bersama yang terpotong", () => {
    // Jum 20 (cuti bersama) · Sab 21? → 21 Maret 2026 jatuh Sabtu,
    // Min 22, Sen 23 + Sel 24 (cuti bersama)
    expect(countLeaveDays("2026-03-20", "2026-03-24", index)).toBe(3);
  });

  test("rentang yang seluruhnya libur menghasilkan nol", () => {
    // pemanggil yang memutuskan apa arti 0 (mis. tolak pengajuan)
    expect(countLeaveDays("2026-08-15", "2026-08-17", index)).toBe(0);
  });

  test("libur nasional menang atas cuti bersama pada tanggal yang sama", () => {
    const bentrok = indexHolidays([
      { holiday_date: "2026-09-01", name: "Libur X", type: "nasional", deducts_leave: false },
      { holiday_date: "2026-09-01", name: "Cuti Bersama X", type: "cuti_bersama", deducts_leave: true },
    ]);
    expect(countLeaveDays("2026-09-01", "2026-09-01", bentrok)).toBe(0);
  });

  test("libur perusahaan diperlakukan sesuai flag-nya", () => {
    const perusahaan = indexHolidays([
      { holiday_date: "2026-09-02", name: "HUT Perusahaan", type: "perusahaan", deducts_leave: false },
    ]);
    expect(countLeaveDays("2026-09-02", "2026-09-02", perusahaan)).toBe(0);
  });
});

// EPIC-036 Fase D — rincian yang dipakai endpoint cuti & pratinjau di form,
// supaya karyawan melihat ALASAN jatahnya tidak terpotong penuh.
describe("describeLeaveDays", () => {
  test("menyebut libur yang tidak memotong jatah", () => {
    const result = describeLeaveDays("2026-08-17", "2026-08-19", index);
    expect(result.totalDays).toBe(2);
    expect(result.excludedHolidays).toEqual([
      { date: "2026-08-17", name: "Hari Kemerdekaan Republik Indonesia" },
    ]);
  });

  test("cuti bersama tidak masuk daftar pengecualian karena tetap memotong", () => {
    const result = describeLeaveDays("2026-03-20", "2026-03-20", index);
    expect(result.totalDays).toBe(1);
    expect(result.excludedHolidays).toEqual([]);
  });

  test("akhir pekan bukan 'libur' — tidak ikut disebut namanya", () => {
    // Sab 15 – Sen 17 Agustus: 2 hari akhir pekan + 1 libur nasional
    const result = describeLeaveDays("2026-08-15", "2026-08-17", index);
    expect(result.totalDays).toBe(0);
    expect(result.excludedHolidays).toEqual([
      { date: "2026-08-17", name: "Hari Kemerdekaan Republik Indonesia" },
    ]);
  });

  test("konsisten dengan countLeaveDays untuk klaster Idul Fitri", () => {
    const result = describeLeaveDays("2026-03-20", "2026-03-24", index);
    expect(result.totalDays).toBe(countLeaveDays("2026-03-20", "2026-03-24", index));
    expect(result.excludedHolidays).toEqual([
      { date: "2026-03-21", name: "Hari Raya Idul Fitri 1447 H" },
    ]);
  });

  test("rentang terbalik menghasilkan nol tanpa melempar", () => {
    const result = describeLeaveDays("2026-08-19", "2026-08-17", index);
    expect(result.totalDays).toBe(0);
    expect(result.excludedHolidays).toEqual([]);
  });
});

describe("isHolidayDayOff (EPIC-079 R1-R3)", () => {
  const row = (type: "nasional" | "cuti_bersama" | "perusahaan", is_day_off?: boolean) => ({
    holiday_date: "2026-12-24", name: "X", type, deducts_leave: type === "cuti_bersama", is_day_off,
  });

  test("staf shift tidak libur di tanggal merah apa pun", () => {
    for (const type of ["nasional", "perusahaan", "cuti_bersama"] as const) {
      expect(isHolidayDayOff(row(type, true), "shift")).toBe(false);
    }
  });

  test("Non-Shift libur di nasional dan perusahaan, cuti bersama hanya bila ditetapkan libur", () => {
    expect(isHolidayDayOff(row("nasional"), "non_shift")).toBe(true);
    expect(isHolidayDayOff(row("perusahaan"), "non_shift")).toBe(true);
    expect(isHolidayDayOff(row("cuti_bersama", false), "non_shift")).toBe(false);
    expect(isHolidayDayOff(row("cuti_bersama"), "non_shift")).toBe(false);
    expect(isHolidayDayOff(row("cuti_bersama", true), "non_shift")).toBe(true);
  });

  test("tipe jadwal tidak diketahui diperlakukan Non-Shift", () => {
    expect(isHolidayDayOff(row("nasional"), null)).toBe(true);
  });
});
