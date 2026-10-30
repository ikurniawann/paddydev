import { describe, expect, it } from "vitest";
import nextConfig from "../../next.config";

/**
 * Menu IAM aktif yang route-nya tidak punya halaman fisik harus dialihkan,
 * supaya sidebar tidak membuka atau prefetch halaman 404 (cek 243 menu staging 30-10).
 */
describe("redirect menu tanpa halaman", () => {
  it("KPI Templates → Konfigurasi KPI, Ecommerce → Pesanan", async () => {
    const rules = (await nextConfig.redirects?.()) ?? [];
    const to = (source: string) => rules.find((r) => r.source === source)?.destination;
    expect(to("/dashboard/hris/kpi-templates")).toBe("/dashboard/hris/kpi-config");
    expect(to("/dashboard/shop")).toBe("/dashboard/shop/orders");
  });
});
