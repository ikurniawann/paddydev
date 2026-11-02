/**
 * TnC "Hari dan Jam Kerja" (EPIC-078 + EPIC-079, 02-11-2026), teks perusahaan
 * apa adanya. Jadwal tetap Non-Shift mengikuti ini: HQ = shift NS (terlambat
 * lewat 09.00), Operations/Warehouse = OPS + OPSSAT (toleransi 10 menit).
 * Sistem shift (Shift 1–3) memakai roster; `notes` = catatan unit kerja,
 * kewajiban menaati jam kerja, dan dua paragraf hari libur.
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
    {
      name: "Karyawan Sistem Shift",
      scope: "Retail Operasional, Host Live, CRM dan Produksi.",
      rows: [
        ["Hari", "Senin – Minggu (libur 1 hari setiap minggu)"],
        ["Shift 1", "08.00 – 16.00 WIB"],
        ["Shift 2", "11.00 – 19.00 WIB"],
        ["Shift 3", "13.30 – 21.30 WIB"],
      ],
    },
  ],
  notes: [
    "Sebagai catatan jam mulai dan berakhirnya shift dapat berbeda sesuai dengan unit kerja masing-masing.",
    "Karyawan wajib menaati ketentuan jam kerja yang telah ditetapkan oleh perusahaan. Apabila terdapat perubahan atau penyesuaian terhadap waktu kerja karena kebutuhan operasional perusahaan, maka perubahan tersebut akan diinformasikan terlebih dahulu kepada karyawan secara layak.",
    "Hari libur untuk karyawan Operations dan Headquarter mengikuti hari libur nasional yang ditetapkan oleh pemerintah Republik Indonesia dan hari libur sesuai dengan shift yang telah ditentukan. Hari cuti bersama tidak secara otomatis menjadi hari libur, kecuali jika ditetapkan demikian oleh perusahaan.",
    "Hari libur untuk karyawan dengan sistem kerja shift diberikan begitu tercapai 40 jam kerja dan tidak mengikuti hari libur nasional atau tanggal merah yang ditetapkan oleh Pemerintah Republik Indonesia. Penetapan hari libur disesuaikan dengan kebijakan internal perusahaan dan/atau jadwal kerja (shift) yang telah ditentukan.",
  ],
} as const;

/** Ringkasan jadwal tetap Non-Shift per kelompok, untuk teks konfirmasi HR. */
export const FIXED_SCHEDULE_SUMMARY =
  "HQ: Senin–Jumat, masuk 08.00–09.00; Warehouse: Senin–Jumat 08.00–16.00, Sabtu 08.00–14.00";
