import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api-client";
import { isDelegateAccessLost, retryUnlessForbidden } from "./access-guard";

const forbidden = (message: string) => new ApiError(message, 403, null);

describe("isDelegateAccessLost", () => {
  it("403 umum = penunjukan dicabut", () => {
    expect(isDelegateAccessLost(forbidden("Insufficient permissions"))).toBe(true);
  });
  it.each(["Toko di luar wilayah Anda", "2 karyawan di luar wilayah Anda", "Anda tidak punya hak mengatur penempatan"])(
    "403 %s bukan akses hilang",
    (message) => expect(isDelegateAccessLost(forbidden(message))).toBe(false)
  );
  it("bukan 403 bukan akses hilang", () => {
    expect(isDelegateAccessLost(new ApiError("Insufficient permissions", 400, null))).toBe(false);
    expect(isDelegateAccessLost(new Error("Insufficient permissions"))).toBe(false);
  });
});

describe("retryUnlessForbidden", () => {
  it("403 tidak dicoba ulang", () => {
    expect(retryUnlessForbidden(0, forbidden("Insufficient permissions"))).toBe(false);
    expect(retryUnlessForbidden(0, forbidden("Toko di luar wilayah Anda"))).toBe(false);
  });
  it("galat lain dicoba ulang 2x seperti bawaan", () => {
    const err = new ApiError("Server error", 500, null);
    expect(retryUnlessForbidden(0, err)).toBe(true);
    expect(retryUnlessForbidden(1, err)).toBe(true);
    expect(retryUnlessForbidden(2, err)).toBe(false);
    expect(retryUnlessForbidden(0, new Error("Jaringan terputus"))).toBe(true);
  });
});
