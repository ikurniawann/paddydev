export const scheduleDelegateKeys = {
  all: ["hris", "schedule-delegates"] as const,
  suggest: (employeeId: string) => ["hris", "schedule-delegates", "suggest", employeeId] as const,
  employees: (q: string) => ["hris", "schedule-delegates", "employees", q] as const,
};

/** Akses Jadwal Toko di Area Karyawan ikut berubah saat pengatur disimpan/dicabut. */
export const SCHEDULE_ACCESS_KEY = ["hris", "schedule-access"] as const;
