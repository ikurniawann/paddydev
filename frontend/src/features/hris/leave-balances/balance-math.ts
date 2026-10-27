import { formatNumber } from "@/lib/format";

/** "9,5" / "9.5" / "-1" → angka; kosong atau bukan angka → null. */
export function parseDays(text: string): number | null {
  const s = text.trim().replace(",", ".");
  if (s === "" || !/^-?\d+(\.\d+)?$/.test(s)) return null;
  return Number(s);
}

/** Kelipatan 0,5 (aturan jatah, penyesuaian, dan sisa bawaan). */
export function isHalfStep(n: number): boolean {
  return Math.abs(n * 2 - Math.round(n * 2)) < 1e-9;
}

/** "9,5", "12", "-1,5". */
export function formatDays(n: number): string {
  return formatNumber(n, 2);
}

export interface BalanceFigures {
  total: number;
  adjustment: number;
  used: number;
  carryDays: number;
  carryUsed: number;
  expiresOn: string | null;
  /** YYYY-MM-DD (WIB). */
  today: string;
}

/**
 * Sisa tersedia pada tanggal D (R4): sisa jatah + sisa bawaan bila tanggal
 * kedaluwarsa ≥ D. Tanpa tanggal kedaluwarsa, sisa bawaan tidak berlaku.
 */
export function previewBalance(f: BalanceFigures) {
  const remaining = f.total + f.adjustment - f.used;
  // A carry-over recalculated below what is used counts as 0, never negative
  // (the server clamps the same way).
  const carryRemaining = Math.max(f.carryDays - f.carryUsed, 0);
  const carryActive = !!f.expiresOn && f.expiresOn >= f.today;
  return { remaining, carryRemaining, carryActive, available: remaining + (carryActive ? carryRemaining : 0) };
}
