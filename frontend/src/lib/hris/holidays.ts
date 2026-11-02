/**
 * Logika murni hari libur — dipakai kalender ESS, monitoring absensi,
 * perhitungan hari cuti, dan (nanti) multiplier lembur. Tanpa akses database
 * supaya bisa diuji dan dipakai ulang di client maupun server.
 *
 * Sumber datanya tabel `hris.public_holidays` (EPIC-036 Fase A), bukan API
 * pihak ketiga: daftar resmi terbit lewat SKB 3 Menteri dan tanggal hijriah
 * bisa digeser pemerintah, jadi HRD harus bisa mengoreksi tanpa deploy.
 *
 * Konvensi tanggal: string kalender polos "YYYY-MM-DD". Semua aritmetika
 * dilakukan di UTC (`T00:00:00Z`) mengikuti pola `shifts.ts` — kolom `date`
 * Postgres diserialisasi bergeser 17:00 hari sebelumnya di TZ WIB, sehingga
 * pemakai wajib menormalisasi ke kunci WIB dulu (lihat `wibDateKey` di
 * AttendanceCalendar) sebelum masuk ke sini.
 */

export type HolidayType = "nasional" | "cuti_bersama" | "perusahaan";

export interface HolidayRow {
  /** "YYYY-MM-DD" */
  holiday_date: string;
  name: string;
  type: HolidayType;
  /** true = hari ini tetap memotong jatah cuti tahunan (cuti bersama) */
  deducts_leave: boolean;
  /** Cuti bersama yang ditetapkan libur oleh perusahaan (EPIC-079 R2). */
  is_day_off?: boolean;
}

/**
 * Apakah libur ini hari libur bagi karyawan bertipe jadwal tertentu
 * (EPIC-079 R1-R3, cermin domain.HolidayDayOff). Staf shift tidak mengikuti
 * tanggal merah: roster yang menentukan. Non-Shift libur di nasional dan
 * perusahaan, dan di cuti bersama hanya bila ditetapkan libur. Tipe tidak
 * diketahui diperlakukan Non-Shift (perilaku sebelum EPIC-079).
 */
export function isHolidayDayOff(holiday: HolidayRow, scheduleType: "shift" | "non_shift" | null | undefined): boolean {
  if (scheduleType === "shift") return false;
  if (holiday.type === "cuti_bersama") return holiday.is_day_off === true;
  return true;
}

/** Peta tanggal → daftar libur pada tanggal itu. */
export type HolidayIndex = ReadonlyMap<string, HolidayRow[]>;

const DATE_ISO = /^\d{4}-\d{2}-\d{2}$/;

function assertDateIso(value: string, label: string): void {
  if (!DATE_ISO.test(value)) {
    throw new Error(`${label} harus berformat YYYY-MM-DD, diterima: ${value}`);
  }
}

function toUtcDate(dateIso: string): Date {
  const date = new Date(`${dateIso}T00:00:00Z`);
  if (Number.isNaN(date.getTime())) {
    throw new Error(`Tanggal tidak valid: ${dateIso}`);
  }
  return date;
}

/**
 * Kelompokkan baris libur per tanggal. Pemanggil sudah menyaring
 * `status = 'aktif'` dan `deleted_at IS NULL` di query — baris draft tidak
 * boleh sampai ke sini.
 */
export function indexHolidays(rows: readonly HolidayRow[]): HolidayIndex {
  const index = new Map<string, HolidayRow[]>();
  for (const row of rows) {
    assertDateIso(row.holiday_date, "holiday_date");
    const existing = index.get(row.holiday_date);
    if (existing) existing.push(row);
    else index.set(row.holiday_date, [row]);
  }
  return index;
}

export function holidaysOn(index: HolidayIndex, dateIso: string): HolidayRow[] {
  assertDateIso(dateIso, "dateIso");
  return index.get(dateIso) ?? [];
}

export function isHoliday(index: HolidayIndex, dateIso: string): boolean {
  return holidaysOn(index, dateIso).length > 0;
}

/**
 * Sabtu & Minggu. Sengaja menyamai perilaku lama `calculateBusinessDays` di
 * endpoint cuti — mengubahnya (mis. jadi 6 hari kerja) adalah keputusan
 * kebijakan tersendiri, bukan efek samping epic ini.
 */
export function isWeekend(dateIso: string): boolean {
  assertDateIso(dateIso, "dateIso");
  const day = toUtcDate(dateIso).getUTCDay();
  return day === 0 || day === 6;
}

/** Seluruh tanggal dalam rentang, inklusif. Akhir sebelum awal → kosong. */
export function eachDateIso(startIso: string, endIso: string): string[] {
  assertDateIso(startIso, "startIso");
  assertDateIso(endIso, "endIso");

  const dates: string[] = [];
  const cursor = toUtcDate(startIso);
  const end = toUtcDate(endIso);
  while (cursor.getTime() <= end.getTime()) {
    dates.push(cursor.toISOString().slice(0, 10));
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }
  return dates;
}

/**
 * Jumlah hari dalam rentang yang benar-benar memotong jatah cuti tahunan.
 *
 * Sebuah tanggal TIDAK memotong bila akhir pekan, atau bila ada libur aktif
 * padanya yang `deducts_leave = false`. Libur nasional dengan sengaja menang
 * atas cuti bersama pada tanggal yang sama — hari itu memang hari libur resmi,
 * dan pilihan yang menguntungkan karyawan adalah yang benar secara aturan.
 *
 * Bisa mengembalikan 0 (rentang yang seluruhnya libur/akhir pekan). Pemanggil
 * yang memutuskan artinya — mis. menolak pengajuan cuti di hari libur.
 */
export function countLeaveDays(
  startIso: string,
  endIso: string,
  index: HolidayIndex
): number {
  return describeLeaveDays(startIso, endIso, index).totalDays;
}

export interface LeaveDaysBreakdown {
  /** Hari yang benar-benar memotong jatah cuti tahunan. */
  totalDays: number;
  /**
   * Libur di dalam rentang yang membuat jatah TIDAK terpotong — dipakai untuk
   * menjelaskan selisihnya ke karyawan ("17 Agustus tidak dihitung"). Akhir
   * pekan sengaja tidak masuk daftar: bukan libur bernama, dan sudah dipahami.
   */
  excludedHolidays: { date: string; name: string }[];
}

/**
 * Sama seperti `countLeaveDays`, tetapi ikut mengembalikan ALASAN pengurangan.
 * Endpoint cuti memakainya untuk pesan penolakan, form memakainya untuk
 * pratinjau sebelum karyawan menekan kirim.
 */
export function describeLeaveDays(
  startIso: string,
  endIso: string,
  index: HolidayIndex
): LeaveDaysBreakdown {
  const excludedHolidays: { date: string; name: string }[] = [];
  let totalDays = 0;

  for (const dateIso of eachDateIso(startIso, endIso)) {
    const exempt = holidaysOn(index, dateIso).filter((holiday) => !holiday.deducts_leave);
    if (exempt.length > 0) {
      // Libur nasional menang atas cuti bersama pada tanggal yang sama —
      // hari itu memang libur resmi, dan tafsir yang menguntungkan karyawan
      // yang benar secara aturan.
      excludedHolidays.push({ date: dateIso, name: exempt[0].name });
      continue;
    }
    if (isWeekend(dateIso)) continue;
    totalDays += 1;
  }

  return { totalDays, excludedHolidays };
}
