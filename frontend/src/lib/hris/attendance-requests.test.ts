import { describe, expect, it } from "vitest";
import {
  attendanceRequestClocks,
  gapHint,
  toAttendanceRequestPayload,
  validateAttendanceRequestForm,
} from "./attendance-requests";

const valid = { date: "2026-10-02", clock_in: "08:00", clock_out: "", reason: "Internet toko mati" };

describe("validateAttendanceRequestForm (pesan sama dengan server)", () => {
  it.each([
    [{ ...valid, date: "" }, "Tanggal wajib diisi"],
    [{ ...valid, date: "2026-10-06" }, "Tanggal pengajuan tidak boleh di masa depan"],
    [{ ...valid, reason: "  Lupa " }, "Keterangan wajib diisi (minimal 5 karakter)"],
    [{ ...valid, clock_in: "" }, "Isi jam masuk dan/atau jam pulang yang diajukan"],
    [valid, ""],
    [{ ...valid, date: "2026-10-05" }, ""],
  ])("%o → %s", (form, msg) => {
    expect(validateAttendanceRequestForm(form, "2026-10-05")).toBe(msg);
  });

  it("keterangan dihitung per code point seperti rune Go, bukan unit UTF-16", () => {
    // 4 emoji = 8 unit UTF-16 tetapi 4 rune → kurang dari 5
    expect(validateAttendanceRequestForm({ ...valid, reason: "😀😀😀😀" }, "2026-10-05")).toBe(
      "Keterangan wajib diisi (minimal 5 karakter)"
    );
    expect(validateAttendanceRequestForm({ ...valid, reason: "😀😀😀😀😀" }, "2026-10-05")).toBe("");
  });

  it("keterangan maksimal 1000 karakter (hitungan UTF-16 seperti server), dihitung setelah dipangkas", () => {
    const max = "a".repeat(1000);
    expect(validateAttendanceRequestForm({ ...valid, reason: ` ${max} ` }, "2026-10-05")).toBe("");
    expect(validateAttendanceRequestForm({ ...valid, reason: `${max}a` }, "2026-10-05")).toBe(
      "Keterangan maksimal 1000 karakter"
    );
    // 501 emoji = 501 rune tetapi 1002 unit UTF-16 → server menolak
    expect(validateAttendanceRequestForm({ ...valid, reason: "😀".repeat(501) }, "2026-10-05")).toBe(
      "Keterangan maksimal 1000 karakter"
    );
  });
});

describe("toAttendanceRequestPayload", () => {
  it("jam kosong menjadi null, keterangan dipangkas, foto hanya bila ada", () => {
    expect(toAttendanceRequestPayload({ ...valid, reason: "  Internet toko mati  " })).toEqual({
      date: "2026-10-02",
      clock_in: "08:00",
      clock_out: null,
      reason: "Internet toko mati",
    });
    expect(toAttendanceRequestPayload(valid, "data:image/png;base64,AA").photo).toBe("data:image/png;base64,AA");
  });
});

describe("label & hak", () => {
  it("ringkasan jam dan petunjuk absen terlewat", () => {
    expect(attendanceRequestClocks({ clock_in: "08:00", clock_out: "17:00" })).toBe("Masuk 08:00 · Pulang 17:00");
    expect(attendanceRequestClocks({ clock_in: null, clock_out: "17:00" })).toBe("Pulang 17:00");
    expect(gapHint("no_clock_out", "08:05")).toBe("Tercatat masuk 08:05 tanpa absen pulang — isi jam pulang yang benar.");
    expect(gapHint("missing", null)).toBe("Tidak ada absen tercatat — isi jam masuk dan jam pulang yang benar.");
  });
});
