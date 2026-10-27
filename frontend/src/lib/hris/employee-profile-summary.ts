/** Ringkasan untuk halaman detail karyawan: masa kerja, absensi bulan ini, saldo cuti. */

/** "2 yr 3 mo" / "5 mo" / "Just joined"; dihitung per bulan kalender. */
export function calculateTenure(joinDate: string, now: Date = new Date()): string {
  const join = new Date(joinDate);
  if (Number.isNaN(join.getTime())) return "-";
  const totalMonths =
    (now.getFullYear() - join.getFullYear()) * 12 + (now.getMonth() - join.getMonth());
  if (totalMonths <= 0) return "Just joined";
  const years = Math.floor(totalMonths / 12);
  const months = totalMonths % 12;
  return years > 0 ? `${years} yr ${months} mo` : `${months} mo`;
}

interface AttendanceLike {
  status?: string | null;
  is_late?: boolean;
  /** Go mengirim kolom numeric sebagai teks ("11.71"). */
  work_hours?: number | string | null;
}

export function summarizeAttendance(rows: AttendanceLike[]) {
  return {
    present: rows.filter((a) => a.status === "present").length,
    late: rows.filter((a) => a.is_late).length,
    absent: rows.filter((a) => a.status === "absent").length,
    totalHours: rows.reduce((sum, a) => sum + (Number(a.work_hours) || 0), 0),
  };
}
