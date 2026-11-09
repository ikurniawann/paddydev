import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/auth";
import { assertFallbackCanDecide } from "./leaves-repo";

// EPIC-088 R9: jalur fallback Next tidak boleh menyetujui pengajuan berlampiran
// tanpa verifikasi HR di Go.
describe("assertFallbackCanDecide", () => {
  it("approve berlampiran → 503", () => {
    try {
      assertFallbackCanDecide("leave-attachments/e1/x.png", "approve");
      expect.unreachable();
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(503);
      expect((e as ApiError).message).toBe("Pengajuan berlampiran diproses oleh layanan utama, coba lagi sebentar");
    }
  });
  it("tanpa lampiran atau reject → lanjut", () => {
    expect(() => assertFallbackCanDecide(null, "approve")).not.toThrow();
    expect(() => assertFallbackCanDecide("  ", "approve")).not.toThrow();
    expect(() => assertFallbackCanDecide("leave-attachments/e1/x.png", "reject")).not.toThrow();
  });
});
