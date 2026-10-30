import { leaveTypeLabel } from "@/lib/hris/leave-types";

/**
 * Kotak keputusan desktop — isi kartu "Perlu Keputusan", tapi berupa DAFTAR
 * yang bisa ditindaklanjuti, bukan sekadar angka. Setiap item membuka
 * halaman yang tepat sebagai jendela; keputusan cuti diambil di halaman Cuti
 * HR sesudah membaca alasan & lampiran (EPIC-072 R6).
 */

export type InboxSectionKey = "cuti" | "po" | "stok";

export interface InboxItem {
  id: string;
  title: string;
  subtitle?: string | null;
  /** Halaman yang dibuka untuk memutus dengan konteks penuh. */
  href: string;
}

export interface InboxSection {
  key: InboxSectionKey;
  label: string;
  total: number;
  items: InboxItem[];
}

export const INBOX_SECTION_IAM: Record<InboxSectionKey, readonly string[]> = {
  cuti: ["hris.workforce", "hris.kepegawaian", "hris"],
  po: ["items.product.approval", "items.raw-material.approval", "items.product.purchasing", "items.raw-material.purchasing"],
  stok: ["items.raw-material", "items", "pos.catalog"],
};

export const INBOX_SECTION_LABEL: Record<InboxSectionKey, string> = {
  cuti: "Pengajuan cuti",
  po: "PO menunggu approval",
  stok: "Stok di bawah minimum",
};

/** Maksimal item per seksi — widget desktop, bukan halaman laporan. */
export const INBOX_ITEM_LIMIT = 5;

export function allowedInboxSections(
  role: string | null | undefined,
  granted: readonly string[]
): InboxSectionKey[] {
  const keys = Object.keys(INBOX_SECTION_IAM) as InboxSectionKey[];
  if (role && ["super_admin", "admin", "direksi"].includes(role)) return keys;
  if (granted.length === 0) return [];
  return keys.filter((key) =>
    INBOX_SECTION_IAM[key].some((prefix) =>
      granted.some((code) => code === prefix || code.startsWith(`${prefix}.`))
    )
  );
}

export function totalInboxCount(sections: InboxSection[]): number {
  return sections.reduce((sum, section) => sum + section.total, 0);
}

/** Rentang tanggal cuti dalam bahasa manusia: "3 hari · 18–20 Sep". */
export function describeLeaveRange(start: string, end: string, days: number): string {
  const fmt = (iso: string) =>
    new Date(`${iso}T00:00:00`).toLocaleDateString("id-ID", { day: "numeric", month: "short" });
  const range = start === end ? fmt(start) : `${fmt(start)}–${fmt(end)}`;
  return `${days} hari · ${range}`;
}

/**
 * Item cuti panel Hari Ini (EPIC-072 R6, sama dengan Go desktop inbox): tanpa
 * Setujui/Tolak langsung — keputusan diambil di halaman Cuti HR sesudah
 * membaca alasan & lampiran; alasan tidak ditampilkan di panel.
 */
export function leaveInboxItem(row: Record<string, unknown>): InboxItem {
  return {
    id: String(row.id),
    title: String(row.full_name ?? "Karyawan"),
    subtitle: `${leaveTypeLabel(String(row.leave_type ?? "Cuti"))} · ${describeLeaveRange(
      String(row.start_date).slice(0, 10),
      String(row.end_date).slice(0, 10),
      Number(row.total_days ?? 1)
    )}`,
    href: "/dashboard/hris/leaves",
  };
}

