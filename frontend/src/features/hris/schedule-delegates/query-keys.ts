export const scheduleDelegateKeys = {
  all: ["hris", "schedule-delegates"] as const,
  // Di luar awalan `all` supaya menyimpan/mencabut tidak memuat ulang saran & pencarian yang sedang terbuka.
  suggest: (employeeId: string) => ["hris", "schedule-delegate-suggest", employeeId] as const,
  employees: (q: string) => ["hris", "schedule-delegate-employees", q] as const,
};

/** Akses Jadwal Toko di Area Karyawan ikut berubah saat pengatur disimpan/dicabut. */
export { SCHEDULE_ACCESS_KEY } from "@/features/hris/schedule-access/query-keys";
