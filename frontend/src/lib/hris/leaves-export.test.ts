import { describe, expect, it } from "vitest";
import { leavesToCsv, type LeaveExportRow } from "./leaves-export";

const row: LeaveExportRow = {
  id: "lv-1",
  leave_type: "sick",
  status: "approved",
  start_date: "2026-06-01",
  end_date: "2026-06-02",
  total_days: 2,
  reason: 'Demam "tinggi"',
  approved_at: "2026-06-01T03:00:00Z",
  rejection_reason: null,
  created_at: "2026-05-31T10:15:00Z",
  employee: {
    full_name: "Budi",
    nip: "EMP-1",
    department: { name: "Bar" },
    job_title: { title: "Barista" },
  },
  approver: { full_name: "Sari", nip: "EMP-9" },
};

describe("leavesToCsv", () => {
  it("header Indonesia + baris ber-quote dengan label jenis/status", () => {
    const [header, line] = leavesToCsv([row]).split("\n");
    expect(header.startsWith("ID Cuti,NIP,Nama Karyawan")).toBe(true);
    expect(line).toContain('"Cuti Sakit"');
    expect(line).toContain('"Disetujui"');
    expect(line).toContain('"Demam ""tinggi"""');
    expect(line).toContain('"Sari (EMP-9)"');
    // 03:00Z = 10.00 WIB
    expect(line).toContain("10.00.00");
  });

  it("kolom kosong jadi '-' dan jenis tak dikenal apa adanya", () => {
    const line = leavesToCsv([
      { ...row, leave_type: "lainnya", employee: null, approver: null, approved_at: null, reason: null },
    ]).split("\n")[1];
    expect(line).toContain('"lainnya"');
    expect(line.match(/"-"/g)?.length).toBeGreaterThanOrEqual(6);
  });
});
