import { describe, expect, it } from "vitest";
import { parseXlsxToMatrix } from "@/lib/spreadsheet/exceljs-safe";
import {
  buildAttendancePdf,
  buildAttendanceXlsx,
  buildPeriodLabel,
  statusLabel,
  type AttendanceReportRow,
  buildAttendanceCsv,
  toDateString,
  toReportRows,
  type AttendanceExportRecord,
} from "./attendance-report";

const contoh = (over: Partial<AttendanceReportRow> = {}): AttendanceReportRow => ({
  date: "2026-08-27",
  employeeName: "Nanda Romdona",
  nip: "TMP-001",
  department: "Kitchen",
  position: "CDP Kitchen",
  clockIn: "2026-08-27T01:58:00Z",
  clockOut: "2026-08-27T08:58:00Z",
  workHours: 7,
  status: "present",
  isLate: false,
  lateMinutes: 0,
  isShortHours: false,
  minWorkHours: null,
  notes: null,
  clockInPhotoPath: null,
  clockOutPhotoPath: null,
  ...over,
});

describe("buildPeriodLabel", () => {
  it("satu bulan penuh jadi ringkas", () => {
    expect(buildPeriodLabel("2026-08-01", "2026-08-31")).toBe("1–31 Agustus 2026");
  });
  it("lintas bulan dalam tahun sama", () => {
    expect(buildPeriodLabel("2026-08-25", "2026-09-05")).toBe("25 Agustus – 5 September 2026");
  });
});

describe("statusLabel", () => {
  it("status dikenal diterjemahkan; tak dikenal apa adanya", () => {
    expect(statusLabel("present")).toBe("Hadir");
    expect(statusLabel("late")).toBe("Terlambat");
    expect(statusLabel("custom")).toBe("custom");
    expect(statusLabel(null)).toBe("—");
  });
});

describe("buildAttendanceXlsx", () => {
  it("workbook memuat judul, periode, dan baris data", async () => {
    const buffer = await buildAttendanceXlsx([contoh()], {
      companyName: "Paddy",
      periodLabel: "1–31 Agustus 2026",
      employeeLabel: "Nanda Romdona",
      generatedAt: new Date("2026-08-28T03:00:00Z"),
    });
    const matrix = await parseXlsxToMatrix(buffer);
    const text = JSON.stringify(matrix);
    expect(text).toContain("Paddy — Rekap Absensi");
    expect(text).toContain("Periode: 1–31 Agustus 2026");
    expect(text).toContain("Nanda Romdona");
    expect(text).toContain("Hadir");
  });

  it("EPIC-067: Kurang Jam & Jam Minimum tepat sebelum Catatan", async () => {
    const meta = { companyName: "P", periodLabel: "p", employeeLabel: null, generatedAt: new Date(0) };
    const buffer = await buildAttendanceXlsx(
      [
        contoh({ isLate: true, lateMinutes: 5, isShortHours: true, minWorkHours: 9, notes: 'kata "kutip"' }),
        contoh({ date: "2026-08-28" }),
      ],
      meta
    );
    const matrix = await parseXlsxToMatrix(buffer);
    const header = matrix.find((row) => row[0] === "Tanggal");
    expect(header?.slice(-4)).toEqual(["Terlambat (menit)", "Kurang Jam", "Jam Minimum", "Catatan"]);
    const data = matrix.filter((row) => row[1] === "Nanda Romdona");
    expect(data[0].slice(-4)).toEqual(["5", "Ya", "9", 'kata "kutip"']);
    expect(data[1].slice(-4)).toEqual(["0", "-", "-", ""]);
  });
});

describe("buildAttendancePdf", () => {
  it("menghasilkan PDF valid tanpa foto (placeholder aman)", async () => {
    const buffer = await buildAttendancePdf(
      [contoh(), contoh({ date: "2026-08-28", isLate: true, lateMinutes: 12, status: "late" })],
      {
        companyName: "Paddy",
        periodLabel: "1–31 Agustus 2026",
        employeeLabel: null,
        generatedAt: new Date("2026-08-28T03:00:00Z"),
      },
      async () => null
    );
    expect(buffer.subarray(0, 5).toString()).toBe("%PDF-");
    expect(buffer.length).toBeGreaterThan(1000);
  });

  it("foto PNG ter-embed (ukuran PDF membesar)", async () => {
    // PNG 1x1 valid
    const png = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
      "base64"
    );
    const tanpaFoto = await buildAttendancePdf([contoh()], {
      companyName: "S", periodLabel: "p", employeeLabel: null, generatedAt: new Date(0),
    }, async () => null);
    const denganFoto = await buildAttendancePdf(
      [contoh({ clockInPhotoPath: "attendance/x/a.png" })],
      { companyName: "S", periodLabel: "p", employeeLabel: null, generatedAt: new Date(0) },
      async () => ({ data: png, mime: "image/png" })
    );
    expect(denganFoto.subarray(0, 5).toString()).toBe("%PDF-");
    expect(denganFoto.length).toBeGreaterThan(tanpaFoto.length);
  });
});

describe("ekspor dari baris query", () => {
  const record = (overrides: Partial<AttendanceExportRecord> = {}): AttendanceExportRecord => ({
    date: "2026-08-27",
    clock_in: "2026-08-27T01:58:00Z",
    clock_out: null,
    clock_in_location: { latitude: -6.2, longitude: 106.8, address: "Kantor" },
    clock_out_location: null,
    work_hours: "8.5",
    break_minutes: null,
    status: "present",
    is_late: true,
    late_minutes: 5,
    is_short_hours: true,
    min_work_hours: "9.00",
    notes: 'kata "kutip"',
    clock_in_photo_url: null,
    clock_out_photo_url: null,
    employee: { full_name: "Budi", nip: "EMP-1", department: { name: "Ops" }, job_title: null },
    ...overrides,
  });

  it("toDateString membaca objek Date dengan komponen lokal", () => {
    expect(toDateString(new Date(2026, 7, 27))).toBe("2026-08-27");
    expect(toDateString("2026-08-27T00:00:00Z")).toBe("2026-08-27");
  });

  it("toReportRows urut per karyawan lalu tanggal", () => {
    const rows = toReportRows([
      record({ date: "2026-08-28", employee: { full_name: "Budi" } }),
      record({ date: "2026-08-27", employee: { full_name: "Ani" } }),
      record({ date: "2026-08-27", employee: { full_name: "Budi" } }),
    ]);
    expect(rows.map((r) => `${r.employeeName} ${r.date}`)).toEqual([
      "Ani 2026-08-27",
      "Budi 2026-08-27",
      "Budi 2026-08-28",
    ]);
    expect(rows[0].workHours).toBe(8.5);
  });

  it("CSV: kolom berkutip, jam WIB, lokasi dengan alamat", () => {
    const [header, line] = buildAttendanceCsv([record()]).split("\n");
    expect(header.startsWith("NIP,Nama Karyawan")).toBe(true);
    expect(line).toContain('"EMP-1","Budi","Ops","-","2026-08-27"');
    expect(line).toMatch(/08[.:]58/);
    expect(line).toContain('"-6.2,106.8 (Kantor)"');
    expect(line).toContain('"Ya","5","Ya","9.00","kata ""kutip"""');
  });

  it("CSV EPIC-067: header kolom baru berurutan; baris tanpa label memakai '-'", () => {
    const [header, line] = buildAttendanceCsv([
      record({ is_late: false, late_minutes: 0, is_short_hours: false, min_work_hours: null, notes: null }),
    ]).split("\n");
    expect(header.endsWith("Keterlambatan (menit),Kurang Jam,Jam Minimum (jam),Catatan")).toBe(true);
    expect(line.endsWith('"Tidak","0","-","-","-"')).toBe(true);
  });

  it("toReportRows membawa is_short_hours dan min_work_hours", () => {
    const [on, off] = toReportRows([record(), record({ is_short_hours: null, min_work_hours: null, date: "2026-08-28" })]);
    expect([on.isShortHours, on.minWorkHours]).toEqual([true, 9]);
    expect([off.isShortHours, off.minWorkHours]).toEqual([false, null]);
  });
});
