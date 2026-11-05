import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LeavesPage } from "./leaves-page";

let access: { data?: { attendance: boolean; leaves: boolean; overtime: boolean } } = {};
const grant = (leaves: boolean) => {
  access = { data: { attendance: false, leaves, overtime: false } };
};

let search = "";
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(search),
}));
const review = vi.fn();
vi.mock("@/features/hris/leaves/mutations", () => ({
  useReviewLeaveAttachment: () => ({ mutate: review, isPending: false }),
}));
vi.mock("@/features/hris/approve-access", () => ({ useHrApproveAccess: () => access }));
vi.mock("@/components/hris/ApprovalButtons", () => ({
  ApprovalButtons: ({ rejectOnly }: { rejectOnly?: boolean }) => (
    <button>{rejectOnly ? "tombol-tolak" : "tombol-keputusan"}</button>
  ),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
let myEmployeeId = "hr-1";
vi.mock("@/features/hris/ess/queries", () => ({ useEssMe: () => ({ data: { employee: { id: myEmployeeId } } }) }));
let listParams: Record<string, unknown> = {};
let rows: Record<string, unknown>[] = [];
const budi = {
  id: "l1",
  employee_id: "e1",
  status: "pending",
  leave_type: "annual",
  start_date: "2026-10-27",
  end_date: "2026-10-28",
  total_days: 2,
  reason: "Acara keluarga di Garut.\nBerangkat Senin pagi.",
  attachment_url: "leave-attachments/e1/undangan.png",
  created_at: "2026-10-26T01:00:00Z",
  employee: { full_name: "Budi Santoso", nip: "N01", department: { name: "Dapur" } },
};
vi.mock("../queries", () => ({
  useLeaveList: (params: Record<string, unknown>) => {
    listParams = params;
    return { isLoading: false, data: rows.length ? rows : [budi] };
  },
}));


afterEach(() => {
  cleanup();
  access = {};
  myEmployeeId = "hr-1";
  rows = [];
  search = "";
  review.mockReset();
});

describe("LeavesPage: verifikasi lampiran (EPIC-088)", () => {
  const awaiting = {
    ...budi,
    id: "l2",
    attachment_review: "awaiting",
    manager_approved_name: "Atasan Budi",
  };

  it("baris menunggu HR: badge, Verifikasi, dan Tolak saja (tanpa Setujui)", () => {
    grant(true);
    rows = [awaiting];
    render(<LeavesPage />);
    expect(screen.getAllByText("Menunggu verifikasi HR").length).toBeGreaterThan(0);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
    expect(screen.getByText("tombol-tolak")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Verifikasi lampiran Budi Santoso" }));
    expect(review).toHaveBeenCalledWith({ leave_id: "l2", action: "verify" }, expect.anything());
  });

  it("Kembalikan wajib alasan lewat dialog", () => {
    grant(true);
    rows = [awaiting];
    render(<LeavesPage />);
    fireEvent.click(screen.getByRole("button", { name: "Kembalikan lampiran Budi Santoso" }));
    const dialog = screen.getByRole("dialog");
    const send = within(dialog).getByRole("button", { name: "Kembalikan ke karyawan" });
    expect((send as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: "Surat buram" } });
    fireEvent.click(send);
    expect(review).toHaveBeenCalledWith({ leave_id: "l2", action: "return", reason: "Surat buram" }, expect.anything());
  });

  it("baris dikembalikan: badge, hanya Tolak (review akhir: tidak macet selamanya)", () => {
    grant(true);
    rows = [{ ...budi, id: "l3", attachment_review: "returned", attachment_return_reason: "Surat buram" }];
    render(<LeavesPage />);
    expect(screen.getAllByText("Lampiran dikembalikan").length).toBeGreaterThan(0);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
    expect(screen.getByText("tombol-tolak")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Verifikasi lampiran/ })).toBeNull();
  });

  it("?review=awaiting dari notifikasi menjadi filter awal", () => {
    grant(true);
    search = "review=awaiting";
    render(<LeavesPage />);
    expect(listParams).toMatchObject({ review: "awaiting", status: undefined });
  });
});

describe("LeavesPage: tombol keputusan mengikuti akses approve (EPIC-067 R14)", () => {
  it("leaves=true: tombol keputusan tampil untuk pengajuan pending", () => {
    grant(true);
    render(<LeavesPage />);
    expect(screen.getByText("tombol-keputusan")).toBeTruthy();
  });

  it("leaves=false: tombol keputusan tidak dirender, daftar tetap tampil", () => {
    grant(false);
    render(<LeavesPage />);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
    expect(screen.getByText("Budi Santoso")).toBeTruthy();
  });

  it("selama memuat atau gagal (data kosong): tombol tidak tampil", () => {
    access = {};
    render(<LeavesPage />);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
  });
});

describe("LeavesPage: detail & lampiran (EPIC-072)", () => {
  it("kartu menampilkan lampiran dengan pratinjau; Detail memuat alasan utuh dan tombol keputusan", () => {
    grant(true);
    render(<LeavesPage />);
    fireEvent.click(screen.getByRole("button", { name: "Lampiran 1 cuti Budi Santoso" }));
    expect(screen.getByAltText("Lampiran 1").getAttribute("src")).toBe(
      "/api/hris/leaves/attachment/leave-attachments/e1/undangan.png");
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Budi Santoso" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText((_, el) => el?.tagName === "P" && el.textContent === "Acara keluarga di Garut.\nBerangkat Senin pagi.")).toBeTruthy();
    expect(within(dialog).getByText("tombol-keputusan")).toBeTruthy();
  });

  it("tanpa akses approve: Detail tetap bisa dibuka, tanpa tombol keputusan", () => {
    grant(false);
    render(<LeavesPage />);
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Budi Santoso" }));
    expect(within(screen.getByRole("dialog")).queryByText("tombol-keputusan")).toBeNull();
  });
});

describe("LeavesPage: pengajuan sendiri (EPIC-072 Review Focus #4)", () => {
  it("pemegang approve tidak melihat tombol keputusan pada cutinya sendiri, di kartu maupun Detail", () => {
    grant(true);
    myEmployeeId = "e1";
    render(<LeavesPage />);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Budi Santoso" }));
    expect(within(screen.getByRole("dialog")).queryByText("tombol-keputusan")).toBeNull();
  });
});

describe("LeavesPage: tahap SPV (EPIC-092 R7)", () => {
  it("pending yang sudah disetujui SPV: lencana menunggu Manager dan nama SPV di Detail", () => {
    grant(true);
    rows = [{ ...budi, spv_approved_at: "2026-10-26T05:00:00Z", spv_approved_name: "SPV Dapur" }];
    render(<LeavesPage />);
    expect(screen.getAllByText("Disetujui SPV — menunggu Manager").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /Detail.*Budi Santoso/ }));
    expect(within(screen.getByRole("dialog")).getByText("Disetujui SPV oleh SPV Dapur")).toBeTruthy();
  });
});
