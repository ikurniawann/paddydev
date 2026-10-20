import { describe, it, expect } from "vitest";
import {
  greeting,
  initials,
  installmentPreview,
  loanPaidPercent,
  requestStatusBadge,
  splitOvertime,
  tenure,
  visibleAnnouncements,
} from "./ess-view";

describe("greeting", () => {
  it("follows WIB hours", () => {
    expect(greeting(new Date("2026-10-04T01:00:00Z"))).toBe("Selamat pagi"); // 08 WIB
    expect(greeting(new Date("2026-10-04T05:00:00Z"))).toBe("Selamat siang"); // 12 WIB
    expect(greeting(new Date("2026-10-04T09:00:00Z"))).toBe("Selamat sore"); // 16 WIB
    expect(greeting(new Date("2026-10-04T13:00:00Z"))).toBe("Selamat malam"); // 20 WIB
  });
});

describe("initials", () => {
  it("takes the first two words", () => {
    expect(initials("siti nur aisyah")).toBe("SN");
    expect(initials("Budi")).toBe("B");
  });
});

describe("tenure", () => {
  const now = new Date("2026-10-04T00:00:00Z");
  it("formats years and months", () => {
    expect(tenure("2024-07-01", now)).toBe("2 thn 3 bln");
    expect(tenure("2026-05-20", now)).toBe("5 bln");
    expect(tenure("2025-10-01", now)).toBe("1 thn");
  });

  it("handles new joiners, future dates and missing values", () => {
    expect(tenure("2026-10-01", now)).toBe("Baru bergabung");
    expect(tenure("2027-01-01", now)).toBe("Baru bergabung");
    expect(tenure(null, now)).toBe("-");
  });
});

describe("requestStatusBadge", () => {
  it("maps known statuses", () => {
    expect(requestStatusBadge("pending").label).toBe("Menunggu");
    expect(requestStatusBadge("paid").label).toBe("Dibayar");
    expect(requestStatusBadge("APPROVED").label).toBe("Disetujui");
    expect(requestStatusBadge("canceled").label).toBe("Dibatalkan");
    expect(requestStatusBadge("rejected").label).toBe("Ditolak");
  });

  it("labels a kasbon already approved by its manager", () => {
    expect(requestStatusBadge("manager_approved")).toEqual({
      label: "Disetujui Manager",
      cls: "bg-indigo-100 text-indigo-700",
    });
  });

  it("passes unknown statuses through", () => {
    expect(requestStatusBadge("draft")).toEqual({ label: "draft", cls: "bg-gray-100 text-gray-600" });
  });
});

describe("loan helpers", () => {
  it("previews an interest-free installment", () => {
    expect(installmentPreview("1000000", "3")).toBe(333333);
    expect(installmentPreview("", "3")).toBeNull();
    expect(installmentPreview("1000", "0")).toBeNull();
  });

  it("computes the paid share", () => {
    expect(loanPaidPercent("250000", "750000")).toBe(25);
    expect(loanPaidPercent(0, 0)).toBe(0);
  });
});

describe("splitOvertime", () => {
  it("separates pending company assignments from history", () => {
    const rows = [
      { id: "a", source: "company", status: "pending" },
      { id: "b", source: "company", status: "approved" },
      { id: "c", source: "employee", status: "pending" },
    ];
    const { assignments, history } = splitOvertime(rows);
    expect(assignments.map((r) => r.id)).toEqual(["a"]);
    expect(history.map((r) => r.id)).toEqual(["b", "c"]);
  });
});

describe("visibleAnnouncements", () => {
  const item = (id: string, created_at: string, extra: Partial<{ is_read: boolean; is_pinned: boolean }> = {}) => ({
    id,
    created_at,
    publish_at: null,
    is_read: false,
    is_pinned: false,
    ...extra,
  });
  const items = [
    item("old", "2026-09-01"),
    item("new", "2026-10-01", { is_read: true }),
    item("pinned", "2026-08-01", { is_pinned: true }),
  ];

  it("keeps pinned first and sorts by date", () => {
    expect(visibleAnnouncements(items, "all", "newest").map((i) => i.id)).toEqual(["pinned", "new", "old"]);
    expect(visibleAnnouncements(items, "all", "oldest").map((i) => i.id)).toEqual(["pinned", "old", "new"]);
  });

  it("filters by read state", () => {
    expect(visibleAnnouncements(items, "read", "newest").map((i) => i.id)).toEqual(["new"]);
    expect(visibleAnnouncements(items, "unread", "newest").map((i) => i.id)).toEqual(["pinned", "old"]);
  });
});
