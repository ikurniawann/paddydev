import { afterEach, describe, expect, it, vi } from "vitest";
import type { NavItem } from "./types";

const pg = vi.hoisted(() => ({ iamDbQueryOne: vi.fn(), isIamDbConfigured: vi.fn(() => true) }));
vi.mock("./pg-client", () => pg);
vi.mock("@/lib/crm/loyalty-features-server", () => ({ getLoyaltyFeatures: vi.fn() }));

import { dropNavHref } from "./get-user-menus";
import { isScheduleDelegate } from "./schedule-delegate";

afterEach(() => {
  vi.clearAllMocks();
  pg.isIamDbConfigured.mockReturnValue(true);
});

const leaf = (href: string, label = href): NavItem => ({ href, label, icon: "clipboard" });

describe("dropNavHref", () => {
  it("membuang item bersarang dan mempertahankan group yang masih punya anak", () => {
    const items: NavItem[] = [
      leaf("/dashboard"),
      {
        href: "#",
        label: "Area Karyawan",
        icon: "users",
        children: [leaf("/dashboard/me/shift-tim"), leaf("/dashboard/me/jadwal-toko")],
      },
    ];
    expect(dropNavHref(items, "/dashboard/me/jadwal-toko")).toEqual([
      leaf("/dashboard"),
      { href: "#", label: "Area Karyawan", icon: "users", children: [leaf("/dashboard/me/shift-tim")] },
    ]);
    // Pohon asal tidak diubah.
    expect(items[1].children).toHaveLength(2);
  });

  it("membuang group yang kosong karena pembuangan, bukan group lain", () => {
    const items: NavItem[] = [
      { href: "#", label: "Toko", icon: "store", children: [leaf("/dashboard/me/jadwal-toko")] },
      { href: "/dashboard/me", label: "Beranda", icon: "home" },
    ];
    expect(dropNavHref(items, "/dashboard/me/jadwal-toko")).toEqual([{ href: "/dashboard/me", label: "Beranda", icon: "home" }]);
  });

  it("membuang item tingkat atas", () => {
    expect(dropNavHref([leaf("/a"), leaf("/b")], "/a")).toEqual([leaf("/b")]);
  });
});

describe("isScheduleDelegate", () => {
  it("true bila ada penunjukan karyawan aktif dengan toko aktif", async () => {
    pg.iamDbQueryOne.mockResolvedValueOnce({ ok: true });
    await expect(isScheduleDelegate("u1")).resolves.toBe(true);
    const [sql, params] = pg.iamDbQueryOne.mock.calls[0];
    expect(params).toEqual(["u1"]);
    expect(sql).toContain("hris.schedule_delegates");
    expect(sql).toContain("e.is_active");
    expect(sql).toContain("e.user_id = $1");
    expect(sql).toContain("b.is_active");
  });

  it("false bila tidak ada baris", async () => {
    pg.iamDbQueryOne.mockResolvedValueOnce({ ok: false });
    await expect(isScheduleDelegate("u2")).resolves.toBe(false);
  });

  it("error query → false dan dicatat", async () => {
    const log = vi.spyOn(console, "error").mockImplementation(() => {});
    pg.iamDbQueryOne.mockRejectedValueOnce(new Error("boom"));
    await expect(isScheduleDelegate("u3")).resolves.toBe(false);
    expect(log).toHaveBeenCalled();
    log.mockRestore();
  });

  it("tanpa database IAM → false tanpa query", async () => {
    pg.isIamDbConfigured.mockReturnValue(false);
    await expect(isScheduleDelegate("u4")).resolves.toBe(false);
    expect(pg.iamDbQueryOne).not.toHaveBeenCalled();
  });
});
