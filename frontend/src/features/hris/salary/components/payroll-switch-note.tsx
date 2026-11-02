"use client";

import { usePayrollSettings } from "@/features/hris/payroll";

/**
 * Saklar global Pengaturan Payroll (EPIC-082 R7). Selama data belum ada atau
 * gagal dimuat, anggap menyala supaya catatan tidak muncul keliru.
 */
export function usePayrollSwitches() {
  const settings = usePayrollSettings().data?.settings;
  return {
    pph21Off: settings?.pph21_enabled === false,
    bpjsTkOff: settings?.bpjs_tk_enabled === false,
    bpjsKesOff: settings?.bpjs_kes_enabled === false,
  };
}

/** Catatan kecil di samping kepesertaan yang dimatikan secara global. */
export function PayrollSwitchNote({ show, block = false }: { show: boolean; block?: boolean }) {
  if (!show) return null;
  return <span className={`${block ? "block " : ""}text-xs text-gray-500`}>Dinonaktifkan di Pengaturan Payroll</span>;
}
