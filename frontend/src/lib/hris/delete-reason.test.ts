import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DeleteCancelledError,
  HRIS_DELETE_REASON_ERROR,
  askDeleteReason,
  currentDeleteReason,
  isValidDeleteReason,
  retryWithReason,
  withDeleteReason,
  withReasonQuery,
} from "./delete-reason";

afterEach(() => currentDeleteReason()?.reject(new DeleteCancelledError()));

describe("delete-reason", () => {
  it("alasan minimal 5 karakter setelah trim", () => {
    expect(isValidDeleteReason("  abcd ")).toBe(false);
    expect(isValidDeleteReason("abcde")).toBe(true);
  });

  it("menambah reason ke query", () => {
    expect(withReasonQuery("/api/hris/shifts/1", "Shift dobel")).toBe("/api/hris/shifts/1?reason=Shift%20dobel");
    expect(withReasonQuery("/api/hris/kpi/targets?id=1", "a b")).toBe("/api/hris/kpi/targets?id=1&reason=a%20b");
  });

  it("askDeleteReason menunggu dialog, lalu bersih", async () => {
    const p = askDeleteReason({ title: "Hapus shift" });
    expect(currentDeleteReason()?.title).toBe("Hapus shift");
    currentDeleteReason()!.resolve("Shift dobel");
    await expect(p).resolves.toBe("Shift dobel");
    expect(currentDeleteReason()).toBeNull();
  });

  it("permintaan baru membatalkan yang lama", async () => {
    const first = askDeleteReason({ title: "A" });
    const second = askDeleteReason({ title: "B" });
    await expect(first).rejects.toBeInstanceOf(DeleteCancelledError);
    currentDeleteReason()!.resolve("Alasan kedua");
    await expect(second).resolves.toBe("Alasan kedua");
  });

  it("withDeleteReason meneruskan alasan ke fungsi hapus", async () => {
    const del = vi.fn(async (id: string, reason: string) => `${id}:${reason}`);
    const run = withDeleteReason("Hapus shift", del)("s1");
    currentDeleteReason()!.resolve("Shift dobel");
    await expect(run).resolves.toBe("s1:Shift dobel");
  });

  it("retryWithReason bertanya hanya bila server minta alasan", async () => {
    const calls: (string | undefined)[] = [];
    const run = retryWithReason("Timpa jadwal", async (reason) => {
      calls.push(reason);
      if (!reason) throw new Error(HRIS_DELETE_REASON_ERROR);
      return "ok";
    });
    await vi.waitFor(() => expect(currentDeleteReason()).not.toBeNull());
    currentDeleteReason()!.resolve("Revisi jadwal");
    await expect(run).resolves.toBe("ok");
    expect(calls).toEqual([undefined, "Revisi jadwal"]);
    await expect(retryWithReason("x", async () => { throw new Error("lain"); })).rejects.toThrow("lain");
    expect(currentDeleteReason()).toBeNull();
  });
});
