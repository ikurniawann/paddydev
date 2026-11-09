"use client";

import { LettersTable } from "./letters-table";

/** HRIS → Kehadiran & Cuti → Surat Peringatan (EPIC-094 R10). */
export function DisciplinaryLettersPage({ canIssue, canRevoke }: { canIssue: boolean; canRevoke: boolean }) {
  return (
    <div className="space-y-5">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Surat Peringatan</h1>
        <p className="text-sm text-gray-500">
          Surat Teguran dan Surat Peringatan 1–3 yang diterbitkan HR. Tidak ada potongan gaji karena keterlambatan.
        </p>
      </div>
      <LettersTable canIssue={canIssue} canRevoke={canRevoke} />
    </div>
  );
}
