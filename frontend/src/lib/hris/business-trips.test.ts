import { describe, expect, it } from "vitest";
import { EMPTY_TRIP_FORM, TRIP_STATUS_META, tripDays, tripRangeLabel, validateTripForm } from "./business-trips";

const form = { start_date: "2026-10-05", end_date: "2026-10-07", destination: "Gudang Cikarang", purpose: "Stock opname" };

describe("Perjalanan Dinas", () => {
  it("menghitung hari inklusif", () => {
    expect(tripDays("2026-10-05", "2026-10-07")).toBe(3);
    expect(tripDays("2026-10-05", "2026-10-05")).toBe(1);
    expect(tripDays("2026-10-07", "2026-10-05")).toBe(0);
    expect(tripDays("", "2026-10-05")).toBe(0);
  });
  it("memvalidasi seperti server", () => {
    const today = "2026-10-05";
    expect(validateTripForm(form, today)).toBe("");
    expect(validateTripForm(EMPTY_TRIP_FORM, today)).toBe("Tanggal mulai dan selesai wajib diisi");
    expect(validateTripForm({ ...form, start_date: "2026-10-04" }, today)).toBe("Tanggal mulai tidak boleh sebelum hari ini");
    expect(validateTripForm({ ...form, end_date: "2026-10-04", start_date: "2026-10-06" }, today)).toBe(
      "Tanggal selesai tidak boleh sebelum tanggal mulai"
    );
    expect(validateTripForm({ ...form, end_date: "2026-11-05" }, today)).toBe("Perjalanan Dinas maksimal 31 hari per pengajuan");
    expect(validateTripForm({ ...form, end_date: "2026-11-04" }, today)).toBe("");
    expect(validateTripForm({ ...form, destination: " ab " }, today)).toBe("Tujuan minimal 3 karakter");
    expect(validateTripForm({ ...form, purpose: "abcd" }, today)).toBe("Keperluan minimal 5 karakter");
  });
  it("label rentang dan status", () => {
    expect(tripRangeLabel("2026-10-05", "2026-10-07")).toBe("5 Okt 2026 – 7 Okt 2026 (3 hari)");
    expect(tripRangeLabel("2026-10-05", "2026-10-05")).toBe("5 Okt 2026 (1 hari)");
    expect(TRIP_STATUS_META.approved.label).toBe("Disetujui");
  });
});
