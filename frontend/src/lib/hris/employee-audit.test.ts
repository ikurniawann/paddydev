import { expect, it } from "vitest";
import { auditActionLabel, auditFieldLabel, auditValue } from "./employee-audit";

it("label tanpa nama field database", () => {
  expect(auditActionLabel("employee.resign")).toBe("Resign");
  expect(auditActionLabel("schedule.bulk_upload")).toBe("Jadwal diunggah");
  expect(auditFieldLabel("full_name")).toBe("Nama Lengkap");
  expect(auditFieldLabel("schedule_type")).toBe("Tipe Jadwal");
  expect(auditValue("schedule_type", "non_shift")).toBe("Non-Shift");
  expect(auditValue("end_date", "2026-10-07")).toBe("07/10/2026");
  expect(auditValue("is_active", false)).toBe("Tidak");
});

it("label aktivasi kembali (EPIC-064)", () => {
  expect(auditActionLabel("employee.rehire")).toBe("Diaktifkan kembali");
  expect(auditFieldLabel("branch_id")).toBe("Cabang Penempatan");
  expect(auditFieldLabel("reactivate_login")).toBe("Akun Login Diaktifkan");
  expect(auditFieldLabel("join_date")).toBe("Tanggal Masuk");
});
