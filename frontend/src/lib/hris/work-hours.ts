/**
 * TnC "Hari dan Jam Kerja" (EPIC-078, 02-11-2026), teks perusahaan apa
 * adanya. Jadwal tetap Non-Shift mengikuti ini: HQ = shift NS (terlambat
 * lewat 09.00), Operations/Warehouse = OPS + OPSSAT (toleransi 10 menit).
 */
export const WORK_HOURS = {
  title: "HARI DAN JAM KERJA",
  groups: [
    {
      name: "Karyawan Headquarter",
      scope:
        "Seluruh staff kantor pusat, baik fungsi inti maupun fungsi pendukung, termasuk namun tidak terbatas pada departemen Marketing, Product Research dan Development, Retail Operation, Finance & Accounting, HRGA, Support dan SCM.",
      rows: [
        ["Hari", "Senin – Jumat"],
        ["Jam masuk kerja", "08.00 – 09.00 WIB"],
        ["Waktu istirahat", "12.00 – 13.00 WIB"],
        ["Jam keluar kerja", "17.00 – 18.00 WIB"],
      ],
    },
    {
      name: "Karyawan Operations",
      scope: "Seluruh staff Warehouse.",
      rows: [
        ["Senin – Jumat", "08.00 – 16.00 WIB"],
        ["Waktu istirahat", "12.00 – 13.00 WIB"],
        ["Sabtu", "08.00 – 14.00 WIB"],
      ],
    },
  ],
} as const;

/** Ringkasan jadwal tetap Non-Shift per kelompok, untuk teks konfirmasi HR. */
export const FIXED_SCHEDULE_SUMMARY =
  "HQ: Senin–Jumat, masuk 08.00–09.00; Warehouse: Senin–Jumat 08.00–16.00, Sabtu 08.00–14.00";
