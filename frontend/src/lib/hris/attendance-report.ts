import PDFDocument from "pdfkit";
import { formatDateTime, formatTime } from "@/lib/format";
import { buildXlsxBuffer } from "@/lib/spreadsheet/exceljs-safe";

/**
 * Pembangkit laporan rekap absensi (permintaan owner 2026-08-28):
 * HR kesulitan membaca CSV — ekspor kini tersedia sebagai Excel yang rapi
 * dan PDF ber-foto selfie. Keduanya mengikuti filter karyawan + periode
 * yang sudah ada di halaman rekap.
 *
 * Mengikuti pola payslip-pdf.ts: dokumen dibangun di server (pdfkit),
 * bukan window.print(), supaya hasilnya sama di perangkat mana pun.
 * Modul ini murni — foto dimuat lewat `photoLoader` yang disuntik route,
 * sehingga bisa diuji tanpa storage.
 */

const MONTHS = [
  "Januari", "Februari", "Maret", "April", "Mei", "Juni",
  "Juli", "Agustus", "September", "Oktober", "November", "Desember",
];

export interface AttendanceReportRow {
  date: string; // YYYY-MM-DD
  employeeName: string;
  nip: string | null;
  department: string | null;
  position: string | null;
  clockIn: string | null; // ISO timestamp
  clockOut: string | null;
  workHours: number | null;
  status: string | null;
  isLate: boolean;
  lateMinutes: number;
  /** EPIC-067: label tersimpan "kurang jam" + jam minimum cabang saat itu. */
  isShortHours: boolean;
  minWorkHours: number | null;
  notes: string | null;
  clockInPhotoPath: string | null;
  clockOutPhotoPath: string | null;
}

export interface AttendanceReportMeta {
  companyName: string;
  /** "1–31 Agustus 2026" atau rentang bebas yang sudah diformat. */
  periodLabel: string;
  /** Nama karyawan bila difilter satu orang; null = semua karyawan. */
  employeeLabel: string | null;
  generatedAt: Date;
}

export function formatTanggalId(iso: string): string {
  const d = new Date(`${iso}T00:00:00+07:00`);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("id-ID", {
    weekday: "short",
    day: "2-digit",
    month: "short",
    year: "numeric",
    timeZone: "Asia/Jakarta",
  });
}

const jamWib = (iso: string | null) => formatTime(iso, "—");

/** "2026-08-01".."2026-08-31" → "1–31 Agustus 2026" (label periode manusiawi). */
function parseCalendarDate(value: string): { y: number; m: number; d: number } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const m = Number(match[2]) - 1;
  const d = Number(match[3]);
  return m >= 0 && m < 12 && d >= 1 && d <= 31 ? { y: Number(match[1]), m, d } : null;
}

export function buildPeriodLabel(startDate: string, endDate: string): string {
  // Tanggal kalender dibaca langsung dari string: lewat Date, getDate() memakai
  // zona waktu mesin dan bergeser sehari di server UTC.
  const s = parseCalendarDate(startDate);
  const e = parseCalendarDate(endDate);
  if (!s || !e) return `${startDate} s.d. ${endDate}`;
  const { d: sd, m: smi, y: sy } = s;
  const { d: ed, m: emi, y: ey } = e;
  const sm = MONTHS[smi];
  const em = MONTHS[emi];
  if (sy === ey && sm === em) return `${sd}–${ed} ${sm} ${sy}`;
  if (sy === ey) return `${sd} ${sm} – ${ed} ${em} ${sy}`;
  return `${sd} ${sm} ${sy} – ${ed} ${em} ${ey}`;
}

const STATUS_LABELS: Record<string, string> = {
  present: "Hadir",
  late: "Terlambat",
  absent: "Tidak Hadir",
  leave: "Cuti/Izin",
  sick: "Sakit",
  holiday: "Libur",
};

export function statusLabel(status: string | null): string {
  if (!status) return "—";
  return STATUS_LABELS[status] ?? status;
}

/* ================================================================== */
/* Excel                                                               */
/* ================================================================== */

export async function buildAttendanceXlsx(
  rows: AttendanceReportRow[],
  meta: AttendanceReportMeta
): Promise<Buffer> {
  const header = [
    "Tanggal", "Nama Karyawan", "NIP", "Departemen", "Jabatan",
    "Jam Masuk", "Jam Pulang", "Jam Kerja", "Status",
    "Terlambat (menit)", "Kurang Jam", "Jam Minimum", "Catatan",
  ];

  const aoa: unknown[][] = [
    [`${meta.companyName} — Rekap Absensi`],
    [`Periode: ${meta.periodLabel}`],
    [`Karyawan: ${meta.employeeLabel ?? "Semua Karyawan"}`],
    [
      `Dicetak: ${meta.generatedAt.toLocaleString("id-ID", {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: "Asia/Jakarta",
      })} WIB`,
    ],
    [],
    header,
    ...rows.map((row) => [
      formatTanggalId(row.date),
      row.employeeName,
      row.nip ?? "-",
      row.department ?? "-",
      row.position ?? "-",
      jamWib(row.clockIn),
      jamWib(row.clockOut),
      row.workHours ?? 0,
      statusLabel(row.status),
      row.isLate ? row.lateMinutes : 0,
      row.isShortHours ? "Ya" : "-",
      row.minWorkHours ?? "-",
      row.notes ?? "",
    ]),
  ];

  // Judul membentang selebar tabel supaya tidak terpotong kolom A.
  const merges = [0, 1, 2, 3].map((r) => ({
    s: { r, c: 0 },
    e: { r, c: header.length - 1 },
  }));
  return buildXlsxBuffer([
    {
      name: "Rekap Absensi",
      rows: aoa as (string | number | null | undefined)[][],
      columnWidths: [18, 26, 20, 18, 18, 10, 11, 9, 12, 16, 11, 13, 30],
      merges,
    },
  ]);
}

/* ================================================================== */
/* PDF ber-foto                                                        */
/* ================================================================== */

const COLOR_TEXT = "#111827";
const COLOR_MUTED = "#6b7280";
const COLOR_LINE = "#d1d5db";
const COLOR_HEAD = "#1f3864";
const COLOR_ZEBRA = "#f3f5fa";
const COLOR_LATE = "#b91c1c";

type Doc = InstanceType<typeof PDFDocument>;

/** Muat foto selfie sebagai buffer siap-embed; null = tanpa foto. */
export type AttendancePhotoLoader = (
  path: string
) => Promise<{ data: Buffer; mime: string } | null>;

// pdfkit hanya menerima JPEG & PNG; selfie webp dilewati dengan placeholder.
const EMBEDDABLE_MIME = /jpe?g|png/i;

// Lebar kolom (total 770pt utk A4 landscape margin 36).
const COLS = [
  { key: "tanggal", label: "Tanggal", w: 78 },
  { key: "nama", label: "Karyawan", w: 128 },
  { key: "masuk", label: "Masuk", w: 46 },
  { key: "pulang", label: "Pulang", w: 46 },
  { key: "jam", label: "Jam", w: 34 },
  { key: "status", label: "Status", w: 74 },
  { key: "fotoMasuk", label: "Foto Masuk", w: 92 },
  { key: "fotoPulang", label: "Foto Pulang", w: 92 },
  { key: "catatan", label: "Catatan", w: 180 },
] as const;

const PAGE_MARGIN = 36;
const ROW_H = 74; // foto 62pt + padding
const HEAD_H = 22;
const PHOTO_H = 62;

function drawTableHeader(doc: Doc, y: number): number {
  let x = PAGE_MARGIN;
  doc.rect(PAGE_MARGIN, y, COLS.reduce((s, c) => s + c.w, 0), HEAD_H).fill(COLOR_HEAD);
  doc.font("Helvetica-Bold").fontSize(8).fillColor("#ffffff");
  for (const col of COLS) {
    doc.text(col.label, x + 4, y + 7, { width: col.w - 8, lineBreak: false });
    x += col.w;
  }
  return y + HEAD_H;
}

function drawDocHeader(doc: Doc, meta: AttendanceReportMeta): number {
  doc.font("Helvetica-Bold").fontSize(14).fillColor(COLOR_TEXT);
  doc.text(`${meta.companyName} — Rekap Absensi`, PAGE_MARGIN, PAGE_MARGIN);
  doc.font("Helvetica").fontSize(9).fillColor(COLOR_MUTED);
  doc.text(
    `Periode: ${meta.periodLabel}   ·   Karyawan: ${meta.employeeLabel ?? "Semua Karyawan"}   ·   Dicetak: ${meta.generatedAt.toLocaleString(
      "id-ID",
      { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Jakarta" }
    )} WIB`,
    PAGE_MARGIN,
    PAGE_MARGIN + 20
  );
  return PAGE_MARGIN + 40;
}

export async function buildAttendancePdf(
  rows: AttendanceReportRow[],
  meta: AttendanceReportMeta,
  photoLoader: AttendancePhotoLoader
): Promise<Buffer> {
  const doc = new PDFDocument({
    size: "A4",
    layout: "landscape",
    margin: PAGE_MARGIN,
    info: { Title: `Rekap Absensi — ${meta.periodLabel}` },
  });
  const chunks: Buffer[] = [];
  doc.on("data", (chunk: Buffer) => chunks.push(chunk));
  const done = new Promise<Buffer>((resolve) =>
    doc.on("end", () => resolve(Buffer.concat(chunks)))
  );

  const pageBottom = doc.page.height - PAGE_MARGIN;
  let y = drawDocHeader(doc, meta);
  y = drawTableHeader(doc, y);

  const drawPhoto = async (
    path: string | null,
    x: number,
    rowY: number,
    w: number
  ) => {
    const boxW = w - 8;
    if (!path) {
      doc.font("Helvetica").fontSize(8).fillColor(COLOR_MUTED);
      doc.text("—", x + 4, rowY + ROW_H / 2 - 4, { width: boxW, align: "center" });
      return;
    }
    const photo = await photoLoader(path);
    if (!photo || !EMBEDDABLE_MIME.test(photo.mime)) {
      doc.font("Helvetica").fontSize(7).fillColor(COLOR_MUTED);
      doc.text("foto tidak dapat", x + 4, rowY + ROW_H / 2 - 9, { width: boxW, align: "center" });
      doc.text("ditampilkan", x + 4, rowY + ROW_H / 2 + 1, { width: boxW, align: "center" });
      return;
    }
    try {
      doc.image(photo.data, x + 4, rowY + (ROW_H - PHOTO_H) / 2, {
        fit: [boxW, PHOTO_H],
        align: "center",
        valign: "center",
      });
    } catch {
      doc.font("Helvetica").fontSize(7).fillColor(COLOR_MUTED);
      doc.text("foto rusak", x + 4, rowY + ROW_H / 2 - 4, { width: boxW, align: "center" });
    }
  };

  for (const [index, row] of rows.entries()) {
    if (y + ROW_H > pageBottom) {
      doc.addPage();
      y = drawTableHeader(doc, PAGE_MARGIN);
    }

    const tableW = COLS.reduce((s, c) => s + c.w, 0);
    if (index % 2 === 1) {
      doc.rect(PAGE_MARGIN, y, tableW, ROW_H).fill(COLOR_ZEBRA);
    }

    let x = PAGE_MARGIN;
    const textY = y + 8;
    const put = (
      text: string,
      w: number,
      opts: { bold?: boolean; color?: string; sub?: string | null } = {}
    ) => {
      doc
        .font(opts.bold ? "Helvetica-Bold" : "Helvetica")
        .fontSize(8)
        .fillColor(opts.color ?? COLOR_TEXT);
      doc.text(text, x + 4, textY, { width: w - 8 });
      if (opts.sub) {
        doc.font("Helvetica").fontSize(7).fillColor(COLOR_MUTED);
        doc.text(opts.sub, x + 4, textY + 11, { width: w - 8 });
      }
      x += w;
    };

    put(formatTanggalId(row.date), COLS[0].w);
    put(row.employeeName, COLS[1].w, {
      bold: true,
      sub: [row.nip, row.department].filter(Boolean).join(" · ") || null,
    });
    put(jamWib(row.clockIn), COLS[2].w);
    put(jamWib(row.clockOut), COLS[3].w);
    put(row.workHours != null ? `${row.workHours}` : "—", COLS[4].w);
    // Tabel sudah penuh: label kurang jam (EPIC-067) menumpang di kolom Status.
    const statusText =
      (row.isLate ? `${statusLabel(row.status)} +${row.lateMinutes}m` : statusLabel(row.status)) +
      (row.isShortHours ? " · kurang jam" : "");
    put(statusText, COLS[5].w, { color: row.isLate ? COLOR_LATE : COLOR_TEXT });

    await drawPhoto(row.clockInPhotoPath, x, y, COLS[6].w);
    x += COLS[6].w;
    await drawPhoto(row.clockOutPhotoPath, x, y, COLS[7].w);
    x += COLS[7].w;

    doc.font("Helvetica").fontSize(8).fillColor(COLOR_TEXT);
    doc.text(row.notes ?? "", x + 4, textY, {
      width: COLS[8].w - 8,
      height: ROW_H - 14,
      ellipsis: true,
    });

    y += ROW_H;
    doc
      .moveTo(PAGE_MARGIN, y)
      .lineTo(PAGE_MARGIN + tableW, y)
      .strokeColor(COLOR_LINE)
      .lineWidth(0.5)
      .stroke();
  }

  if (rows.length === 0) {
    doc.font("Helvetica").fontSize(10).fillColor(COLOR_MUTED);
    doc.text("Tidak ada data absensi pada periode ini.", PAGE_MARGIN, y + 16);
  }

  doc.end();
  return done;
}

/* ================================================================== */
/* Sumber data ekspor + CSV                                            */
/* ================================================================== */

interface AttendanceLocation {
  latitude: number;
  longitude: number;
  address?: string | null;
}

/** Baris hris.attendance + join karyawan seperti dipilih route ekspor. */
export interface AttendanceExportRecord {
  date: string | Date;
  clock_in: string | null;
  clock_out: string | null;
  clock_in_location: AttendanceLocation | null;
  clock_out_location: AttendanceLocation | null;
  work_hours: string | number | null;
  break_minutes: number | null;
  status: string | null;
  is_late: boolean | null;
  late_minutes: number | null;
  is_short_hours: boolean | null;
  min_work_hours: string | number | null;
  notes: string | null;
  clock_in_photo_url: string | null;
  clock_out_photo_url: string | null;
  employee: {
    full_name?: string;
    nip?: string | null;
    department?: { name?: string | null } | null;
    job_title?: { title?: string | null } | null;
  } | null;
}

/**
 * Driver pg bisa mengembalikan kolom date sebagai objek Date: normalkan ke
 * YYYY-MM-DD memakai komponen lokal (toISOString bisa mundur sehari).
 */
export function toDateString(value: string | Date): string {
  if (value instanceof Date) {
    const m = String(value.getMonth() + 1).padStart(2, "0");
    const d = String(value.getDate()).padStart(2, "0");
    return `${value.getFullYear()}-${m}-${d}`;
  }
  return String(value).slice(0, 10);
}

/** Baris ekspor → baris laporan, urut per karyawan lalu tanggal. */
export function toReportRows(records: readonly AttendanceExportRecord[]): AttendanceReportRow[] {
  const rows = records.map((r) => ({
    date: toDateString(r.date),
    employeeName: r.employee?.full_name || "-",
    nip: r.employee?.nip ?? null,
    department: r.employee?.department?.name ?? null,
    position: r.employee?.job_title?.title ?? null,
    clockIn: r.clock_in ?? null,
    clockOut: r.clock_out ?? null,
    workHours: r.work_hours == null ? null : Number(r.work_hours),
    status: r.status ?? null,
    isLate: Boolean(r.is_late),
    lateMinutes: Number(r.late_minutes) || 0,
    isShortHours: Boolean(r.is_short_hours),
    minWorkHours: r.min_work_hours == null ? null : Number(r.min_work_hours),
    notes: r.notes ?? null,
    clockInPhotoPath: r.clock_in_photo_url ?? null,
    clockOutPhotoPath: r.clock_out_photo_url ?? null,
  }));
  return rows.sort((a, b) =>
    a.employeeName === b.employeeName
      ? a.date.localeCompare(b.date)
      : a.employeeName.localeCompare(b.employeeName)
  );
}

const CSV_HEADERS = [
  "NIP",
  "Nama Karyawan",
  "Departemen",
  "Jabatan",
  "Tanggal",
  "Clock In",
  "Clock Out",
  "Lokasi Clock In",
  "Lokasi Clock Out",
  "Jam Kerja (jam)",
  "Istirahat (menit)",
  "Status",
  "Terlambat",
  "Keterlambatan (menit)",
  "Kurang Jam",
  "Jam Minimum (jam)",
  "Catatan",
];

function csvLocation(location: AttendanceLocation | null): string {
  if (!location) return "-";
  const address = location.address ? ` (${location.address})` : "";
  return `${location.latitude},${location.longitude}${address}`;
}

/** CSV ekspor absensi (urutan baris mengikuti query, terbaru dulu). */
export function buildAttendanceCsv(records: readonly AttendanceExportRecord[]): string {
  const lines = records.map((record) =>
    [
      record.employee?.nip || "-",
      record.employee?.full_name || "-",
      record.employee?.department?.name || "-",
      record.employee?.job_title?.title || "-",
      toDateString(record.date),
      formatDateTime(record.clock_in),
      formatDateTime(record.clock_out),
      csvLocation(record.clock_in_location),
      csvLocation(record.clock_out_location),
      record.work_hours || 0,
      record.break_minutes || 0,
      record.status || "-",
      record.is_late ? "Ya" : "Tidak",
      record.late_minutes || 0,
      record.is_short_hours ? "Ya" : "-",
      record.min_work_hours ?? "-",
      record.notes || "-",
    ]
      .map((field) => `"${String(field).replace(/"/g, '""')}"`)
      .join(",")
  );
  return [CSV_HEADERS.join(","), ...lines].join("\n");
}
