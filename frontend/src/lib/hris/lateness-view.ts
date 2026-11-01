import type { LatenessCode, LatenessNext } from "@/features/hris/lateness/types";

/**
 * Tampilan info keterlambatan (EPIC-077). Batas angka tingkat datang dari
 * server (`rules`, `level`, `next`); di sini hanya warna, teks, dan
 * ketentuan perusahaan 02-11-2026.
 */

export interface LatenessTone {
  /** Badge tingkat. */
  badge: string;
  /** Segmen aktif tangga tingkat. */
  bar: string;
  /** Angka besar. */
  text: string;
  /** Latar kartu/chip lembut. */
  soft: string;
}

export const LATENESS_TONE: Record<LatenessCode, LatenessTone> = {
  none: {
    badge: "bg-emerald-100 text-emerald-800 ring-emerald-200",
    bar: "bg-emerald-500",
    text: "text-emerald-700",
    soft: "border-emerald-200 bg-emerald-50/70",
  },
  teguran: {
    badge: "bg-amber-100 text-amber-800 ring-amber-200",
    bar: "bg-amber-400",
    text: "text-amber-700",
    soft: "border-amber-200 bg-amber-50/70",
  },
  sp1: {
    badge: "bg-orange-100 text-orange-800 ring-orange-200",
    bar: "bg-orange-500",
    text: "text-orange-700",
    soft: "border-orange-200 bg-orange-50/70",
  },
  sp2: {
    badge: "bg-red-100 text-red-800 ring-red-200",
    bar: "bg-red-500",
    text: "text-red-700",
    soft: "border-red-200 bg-red-50/70",
  },
  sp3: {
    badge: "bg-rose-900 text-white ring-rose-950",
    bar: "bg-rose-900",
    text: "text-rose-900",
    soft: "border-rose-300 bg-rose-50",
  },
};

/** Label pendek tingkat untuk chip & tangga. */
export const LATENESS_SHORT: Record<LatenessCode, string> = {
  none: "Tepat waktu",
  teguran: "Teguran",
  sp1: "SP 1",
  sp2: "SP 2",
  sp3: "SP 3",
};

/** Rentang pendek di bawah tangga tingkat. */
export const LATENESS_STEP_RANGE: Record<LatenessCode, string> = {
  none: "0",
  teguran: "1–2",
  sp1: "3–4",
  sp2: "5–6",
  sp3: ">6",
};

export const LATENESS_CODES: LatenessCode[] = ["none", "teguran", "sp1", "sp2", "sp3"];

/** "2 kali terlambat lagi → Surat Peringatan 2"; null di tingkat terakhir. */
export function nextLevelHint(next: LatenessNext | null): string | null {
  if (!next) return null;
  return `${next.remaining} kali terlambat lagi → ${next.label}`;
}

/** 45 → "45 mnt", 65 → "1 jam 5 mnt", 120 → "2 jam". */
export function formatLateMinutes(minutes: number): string {
  const m = Math.max(0, Math.round(minutes));
  if (m < 60) return `${m} mnt`;
  const h = Math.floor(m / 60);
  const rest = m % 60;
  return rest ? `${h} jam ${rest} mnt` : `${h} jam`;
}

export const LATENESS_DISCLAIMER = "Informasi pengingat. Surat peringatan resmi diterbitkan oleh HR.";

/** Ketentuan perusahaan 02-11-2026, apa adanya. */
export const LATENESS_POLICY = {
  title: "KETERLAMBATAN DAN ABSENSI",
  lead: "Jika karyawan datang absensi lebih dari jadwal yang ditentukan, maka akan mengikuti ketentuan sebagai berikut:",
  levels: [
    "Jumlah Keterlambatan 0 – 2x dalam 1 bulan diberikan teguran lisan dan tertulis",
    "Jumlah Keterlambatan 3x – 4x dalam 1 bulan diberikan surat peringatan 1",
    "Jumlah Keterlambatan 5x – 6x dalam 1 bulan diberikan surat peringatan 2",
    "Jumlah Keterlambatan di atas 6x dalam 1 bulan diberikan surat peringatan 3 dan/atau Pemutusan Hubungan Kerja.",
  ],
  intro:
    "Seluruh karyawan melakukan absensi melalui website ERP PADDY (dapat berubah sesuai HRIS yang digunakan) dengan menggunakan device tablet yang telah disiapkan kantor.",
  steps: [
    "Pastikan kamu sudah terdaftar di website absensi PADDY.",
    "Pastikan kamu berada dalam area Lokasi kerja yang telah ditentukan.",
    "Di Fitur “Area Karyawan” klik “check-in” saat jam masuk, dan “check-out” saat jam kerja sudah selesai.",
  ],
};
