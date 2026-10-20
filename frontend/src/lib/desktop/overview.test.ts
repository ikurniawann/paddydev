import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const queryMock = vi.fn();
const queryOneMock = vi.fn();
vi.mock("@/lib/db", () => ({
  query: (...args: unknown[]) => queryMock(...args),
  queryOne: (...args: unknown[]) => queryOneMock(...args),
}));

import { buildDesktopOverview, todayJakarta } from "./overview";

beforeEach(() => {
  queryMock.mockReset();
  queryOneMock.mockReset();
});

describe("todayJakarta", () => {
  it("menggeser tanggal ke WIB", () => {
    // 23:30 UTC = 06:30 WIB hari BERIKUTNYA — inilah alasan helper ini ada.
    expect(todayJakarta(new Date("2026-07-20T23:30:00Z"))).toBe("2026-07-21");
    expect(todayJakarta(new Date("2026-07-20T10:00:00Z"))).toBe("2026-07-20");
  });
});

describe("buildDesktopOverview", () => {
  // Pin the clock so the test's todayJakarta() and the module's cannot straddle WIB midnight.
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-04T05:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("mengisi semua seksi saat query sehat", async () => {
    queryMock.mockResolvedValue([]); // sales rows & stok rows kosong tapi valid
    queryOneMock.mockResolvedValue({
      aktif: 19, hadir: 14, terlambat: 2, cuti: 1,
      cuti_p: 0, lembur: 1, pinjaman: 2, po_draft: 4, kandidat: 5,
      member: 12, xp: 4520, redeem: 3,
    });

    const overview = await buildDesktopOverview();
    expect(overview.gagal).toEqual([]);
    expect(overview.timHariIni).toMatchObject({ aktif: 19, hadir: 14 });
    // belum = aktif - hadir - cuti, tidak boleh negatif
    expect(overview.timHariIni?.belum).toBe(19 - 14 - 1);
    expect(overview.pulsaBisnis?.tujuhHari).toHaveLength(7);
  });

  it("menghitung pinjaman menunggu HR (manager_approved atau pending tanpa manager)", async () => {
    queryMock.mockResolvedValue([]);
    queryOneMock.mockResolvedValue({ aktif: 1, hadir: 1, terlambat: 0, cuti: 0 });
    await buildDesktopOverview();
    const sqls = queryOneMock.mock.calls.map((c) => String(c[0])).filter((q) => q.includes("hris.loans"));
    expect(sqls).toHaveLength(1);
    expect(sqls[0]).toContain("status = 'manager_approved' OR (status = 'pending' AND manager_approver_id IS NULL)");
  });

  it("satu seksi gagal tidak menjatuhkan yang lain", async () => {
    queryMock.mockRejectedValue(new Error("tabel pos mati"));
    queryOneMock.mockResolvedValue({ aktif: 10, hadir: 4, terlambat: 0, cuti: 0 });

    const overview = await buildDesktopOverview();
    expect(overview.gagal).toContain("pulsaBisnis");
    expect(overview.gagal).toContain("stokMenipis");
    expect(overview.pulsaBisnis).toBeNull();
    expect(overview.timHariIni).not.toBeNull();
  });

  it("belum tidak pernah negatif meski data janggal", async () => {
    queryMock.mockResolvedValue([]);
    // hadir > aktif bisa terjadi bila ada absensi milik karyawan nonaktif
    queryOneMock.mockResolvedValue({ aktif: 5, hadir: 9, terlambat: 0, cuti: 0 });
    const overview = await buildDesktopOverview();
    expect(overview.timHariIni?.belum).toBe(0);
  });

  it("rata-rata nol saat tidak ada pesanan (bukan NaN)", async () => {
    queryMock.mockResolvedValue([]);
    queryOneMock.mockResolvedValue({});
    const overview = await buildDesktopOverview();
    expect(overview.pulsaBisnis?.hariIni.rataRata).toBe(0);
    expect(Number.isNaN(overview.pulsaBisnis?.hariIni.rataRata)).toBe(false);
  });

  it("minggu lalu = omzet hari yang sama H-7, terpisah dari sparkline", async () => {
    const today = todayJakarta();
    const h7 = new Date(new Date(`${today}T00:00:00Z`).getTime() - 7 * 86_400_000)
      .toISOString()
      .slice(0, 10);
    const kemarin = new Date(new Date(`${today}T00:00:00Z`).getTime() - 86_400_000)
      .toISOString()
      .slice(0, 10);
    queryMock.mockImplementation(async (sql: string) => {
      if (typeof sql === "string" && sql.includes("pos.pos_orders")) {
        return [
          { tanggal: h7, omzet: 750_000, pesanan: 5 },
          { tanggal: kemarin, omzet: 400_000, pesanan: 4 },
          { tanggal: today, omzet: 900_000, pesanan: 6 },
        ];
      }
      return [];
    });
    queryOneMock.mockResolvedValue({});

    const overview = await buildDesktopOverview();
    expect(overview.pulsaBisnis?.mingguLalu).toEqual({ omzet: 750_000, pesanan: 5 });
    expect(overview.pulsaBisnis?.kemarin).toEqual({ omzet: 400_000, pesanan: 4 });
    // H-7 bukan bagian sparkline: 7 titik terakhir dimulai H-6.
    const hari = overview.pulsaBisnis!.tujuhHari;
    expect(hari).toHaveLength(7);
    expect(hari.some((h) => h.tanggal === h7)).toBe(false);
  });

  it("sparkline selalu 7 titik berurutan dan diakhiri hari ini", async () => {
    queryMock.mockResolvedValue([]);
    queryOneMock.mockResolvedValue({});
    const overview = await buildDesktopOverview();
    const hari = overview.pulsaBisnis!.tujuhHari;
    expect(hari).toHaveLength(7);
    expect(hari[6].tanggal).toBe(todayJakarta());
    for (let i = 1; i < 7; i++) {
      expect(new Date(hari[i].tanggal) > new Date(hari[i - 1].tanggal)).toBe(true);
    }
  });
});
