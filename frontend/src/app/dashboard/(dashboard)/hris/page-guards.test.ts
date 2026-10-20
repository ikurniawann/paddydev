// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

// EPIC-057: halaman detail Rekrutmen/Live Monitoring, dashboard rekrutmen dan
// konfigurasi KPI dijaga menu yang sama dengan API-nya (bukan hanya sesi).
const requireIamPage = vi.hoisted(() => vi.fn());
vi.mock("@/lib/auth/require-user", () => ({ requireIamPage }));
vi.mock("next/navigation", () => ({ notFound: vi.fn() }));
vi.mock("@/features/hris/candidates", () => ({ CandidateDetailPage: () => null }));
vi.mock("@/features/hris/live-monitoring", () => ({ LiveMonitorDetailPage: () => null }));
vi.mock("@/features/hris/dashboard", () => ({ RecruitmentDashboardPage: () => null }));
vi.mock("@/features/hris/kpi", () => ({ KpiConfigPage: () => null }));

beforeEach(() => requireIamPage.mockReset());

describe("page guards", () => {
  it("detail kandidat butuh menu Rekrutmen", async () => {
    const { default: Page } = await import("./candidates/[id]/page");
    await Page();
    expect(requireIamPage).toHaveBeenCalledWith(["hris.recruitment"], "/dashboard/me");
  });

  it("detail live monitoring butuh menu Rekrutmen", async () => {
    const { default: Page } = await import("./live-monitoring/[type]/[id]/page");
    await Page({ params: Promise.resolve({ type: "psikotes", id: "s1" }) });
    expect(requireIamPage).toHaveBeenCalledWith(["hris.recruitment"], "/dashboard/me");
  });

  it("dashboard rekrutmen butuh menu Rekrutmen atau Analytics", async () => {
    const { default: Page } = await import("../rekrutmen/page");
    await Page();
    expect(requireIamPage).toHaveBeenCalledWith(["hris.recruitment", "hris.insights.analytics"], "/dashboard/me");
  });

  it("konfigurasi KPI butuh menu pengelola Kinerja, bukan Review/Tugas Departemen karyawan", async () => {
    const { default: Page } = await import("./kpi-config/page");
    await Page();
    expect(requireIamPage).toHaveBeenCalledWith(
      ["hris.performance.kpi-scorecard", "hris.performance.kpi-config"],
      "/dashboard/me"
    );
  });
});
