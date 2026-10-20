/**
 * Logika tampilan Employee Self Service (tanpa React): sapaan, masa kerja,
 * badge status pengajuan, pratinjau cicilan, dan urutan feed pengumuman.
 */

/** Sapaan berdasar jam WIB. */
export function greeting(now = new Date()): string {
  const h = new Date(now.getTime() + 7 * 3600_000).getUTCHours();
  if (h < 11) return "Selamat pagi";
  if (h < 15) return "Selamat siang";
  if (h < 18) return "Selamat sore";
  return "Selamat malam";
}

/** "Siti Nur Aisyah" → "SN". */
export function initials(name: string): string {
  return name
    .split(" ")
    .slice(0, 2)
    .map((word) => word[0]?.toUpperCase() ?? "")
    .join("");
}

/** Masa kerja ringkas: "2 thn 3 bln", "5 bln", atau "Baru bergabung". */
export function tenure(joinDate: string | null, now = new Date()): string {
  if (!joinDate) return "-";
  const start = new Date(`${joinDate.slice(0, 10)}T00:00:00Z`);
  const months = Math.max(
    0,
    (now.getUTCFullYear() - start.getUTCFullYear()) * 12 + (now.getUTCMonth() - start.getUTCMonth())
  );
  const years = Math.floor(months / 12);
  const rest = months % 12;
  if (!years && !rest) return "Baru bergabung";
  return [years > 0 ? `${years} thn` : "", rest > 0 ? `${rest} bln` : ""].filter(Boolean).join(" ");
}

export interface StatusBadge {
  label: string;
  cls: string;
}

/** Badge status pengajuan gabungan (cuti/lembur/pinjaman) di beranda. */
export function requestStatusBadge(status: string): StatusBadge {
  const s = status.toLowerCase();
  if (["pending", "menunggu"].includes(s)) {
    return { label: "Menunggu", cls: "bg-amber-100 text-amber-700" };
  }
  if (s === "manager_approved") {
    return { label: "Disetujui Manager", cls: "bg-indigo-100 text-indigo-700" };
  }
  if (["approved", "disetujui", "paid", "confirmed"].includes(s)) {
    return { label: s === "paid" ? "Dibayar" : "Disetujui", cls: "bg-green-100 text-green-700" };
  }
  if (["rejected", "ditolak", "cancelled", "canceled"].includes(s)) {
    return { label: s.startsWith("cancel") ? "Dibatalkan" : "Ditolak", cls: "bg-red-100 text-red-700" };
  }
  return { label: status, cls: "bg-gray-100 text-gray-600" };
}

/** Perkiraan cicilan tanpa bunga; null bila jumlah atau tenor kosong. */
export function installmentPreview(principal: string | number, tenor: string | number): number | null {
  const p = Number(principal);
  const t = Number(tenor);
  if (!p || !t) return null;
  return Math.round(p / t);
}

/** Persen terbayar dari total pinjaman (0-100). */
export function loanPaidPercent(paid: string | number, remaining: string | number): number {
  const total = Number(paid) + Number(remaining);
  if (total <= 0) return 0;
  return Math.min(100, Math.round((Number(paid) / total) * 100));
}

/** Penugasan perusahaan yang masih pending dipisah dari riwayat. */
export function splitOvertime<T extends { source: string; status: string }>(rows: readonly T[]) {
  const isAssignment = (row: T) => row.source === "company" && row.status === "pending";
  return {
    assignments: rows.filter(isAssignment),
    history: rows.filter((row) => !isAssignment(row)),
  };
}

export type AnnouncementReadFilter = "all" | "unread" | "read";
export type AnnouncementSort = "newest" | "oldest";

interface FeedLike {
  is_read: boolean;
  is_pinned: boolean;
  publish_at: string | null;
  created_at: string;
}

/** Filter dibaca/belum lalu urutkan; pengumuman disematkan tetap di atas. */
export function visibleAnnouncements<T extends FeedLike>(
  items: readonly T[],
  filter: AnnouncementReadFilter,
  order: AnnouncementSort
): T[] {
  const time = (item: T) => new Date(item.publish_at ?? item.created_at).getTime();
  return items
    .filter((item) => (filter === "unread" ? !item.is_read : filter === "read" ? item.is_read : true))
    .sort((a, b) => {
      if (a.is_pinned !== b.is_pinned) return a.is_pinned ? -1 : 1;
      const diff = time(b) - time(a);
      return order === "newest" ? diff : -diff;
    });
}
