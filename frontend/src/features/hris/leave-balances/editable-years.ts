import { todayWib } from "@/lib/dates";
import { previousYearEditableUntil } from "./balance-math";
import { useLeaveSettings } from "./queries";

/**
 * Tahun saldo yang boleh diubah HR, sama dengan backend `balanceYears`: tahun
 * berjalan dan tahun depan, serta tahun lalu sampai tanggal kedaluwarsa sisa
 * bawaan tahun berjalan (I3). Pengaturan hanya dibaca bila `canUpdate`; selama
 * belum termuat, tahun lalu tetap tertutup.
 */
export function useEditableBalanceYears(canUpdate: boolean) {
  const today = todayWib();
  const thisYear = Number(today.slice(0, 4));
  const settings = useLeaveSettings(canUpdate).data;
  const previousUntil = settings ? previousYearEditableUntil(settings, thisYear) : null;
  const previousOpen = previousUntil !== null && today <= previousUntil;
  return {
    thisYear,
    /** Batas koreksi tahun lalu (YYYY-MM-DD) bila masih terbuka, selain itu null. */
    previousUntil: previousOpen ? previousUntil : null,
    canEdit: (year: number) => canUpdate && (year >= thisYear || (year === thisYear - 1 && previousOpen)),
  };
}
