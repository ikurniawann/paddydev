import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LettersTable } from "./letters-table";
import { letter, renderWithClient } from "./test-setup";

const api = vi.hoisted(() => ({
  fetchLetters: vi.fn(),
  fetchMyLetters: vi.fn(),
  fetchLetterLateness: vi.fn(),
  issueLetter: vi.fn(),
  revokeLetter: vi.fn(),
  uploadLetterAttachment: vi.fn(),
}));
vi.mock("../api", async (importOriginal) => ({ ...(await importOriginal<typeof import("../api")>()), ...api }));
vi.mock("@/features/hris/placements/api", () => ({ searchPlacements: vi.fn().mockResolvedValue({ rows: [], total: 0 }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

beforeEach(() => {
  Object.values(api).forEach((fn) => fn.mockReset());
  api.fetchLetters.mockImplementation(async ({ includeRevoked }: { includeRevoked?: boolean }) =>
    includeRevoked
      ? [
          letter({ attachment_path: "disciplinary-letters/e1/1-abcdef0123456789.pdf" }),
          letter({ id: "l2", letter_type: "teguran", label: "Surat Teguran", revoked_at: "2026-11-06T01:00:00Z", revoke_reason: "Salah orang", revoked_by_name: "HR Admin" }),
        ]
      : [letter({ attachment_path: "disciplinary-letters/e1/1-abcdef0123456789.pdf" })]
  );
  api.revokeLetter.mockResolvedValue(letter({ revoked_at: "2026-11-06T01:00:00Z", revoke_reason: "Salah orang" }));
});
afterEach(cleanup);

describe("LettersTable (EPIC-094)", () => {
  it("menampilkan surat dengan tanggal, catatan, lampiran, dan penerbit", async () => {
    renderWithClient(<LettersTable canIssue canRevoke />);
    const row = (await screen.findByText("Surat Peringatan 1")).closest("tr")!;
    expect(within(row).getByText("Ani Wijaya")).toBeTruthy();
    expect(within(row).getByText("05-11-2026")).toBeTruthy();
    expect(within(row).getByText("Terlambat 3 kali")).toBeTruthy();
    expect(within(row).getByText("HR Admin")).toBeTruthy();
    expect(within(row).getByRole("link", { name: "Lihat lampiran" }).getAttribute("href")).toBe(
      "/api/hris/disciplinary-letters/attachment/disciplinary-letters/e1/1-abcdef0123456789.pdf"
    );
    expect(screen.getByRole("button", { name: "Terbitkan surat" })).toBeTruthy();
  });

  it("surat dibatalkan tampil dengan badge dan alasannya bila dicentang", async () => {
    renderWithClient(<LettersTable canIssue canRevoke />);
    await screen.findByText("Surat Peringatan 1");
    expect(screen.queryByText("Surat Teguran")).toBeNull();
    fireEvent.click(screen.getByLabelText("Tampilkan yang dibatalkan"));
    const row = (await screen.findByText("Surat Teguran")).closest("tr")!;
    expect(within(row).getByText("Dibatalkan")).toBeTruthy();
    expect(within(row).getByText("Alasan: Salah orang")).toBeTruthy();
    expect(within(row).queryByRole("button", { name: "Batalkan" })).toBeNull();
    expect(api.fetchLetters).toHaveBeenLastCalledWith({ employeeId: undefined, includeRevoked: true });
  });

  it("batalkan surat dengan alasan wajib", async () => {
    renderWithClient(<LettersTable canIssue canRevoke />);
    const row = (await screen.findByText("Surat Peringatan 1")).closest("tr")!;
    fireEvent.click(within(row).getByRole("button", { name: "Batalkan" }));
    const confirm = await screen.findByRole("button", { name: "Batalkan surat" });
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Alasan pembatalan"), { target: { value: "Salah orang" } });
    fireEvent.click(confirm);
    await waitFor(() => expect(api.revokeLetter).toHaveBeenCalledWith("l1", "Salah orang"));
  });

  it("tanpa hak: tidak ada tombol Terbitkan dan Batalkan; dari profil kolom karyawan disembunyikan", async () => {
    renderWithClient(<LettersTable employeeId="e1" canIssue={false} canRevoke={false} />);
    await screen.findByText("Surat Peringatan 1");
    expect(screen.queryByRole("button", { name: "Terbitkan surat" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Batalkan" })).toBeNull();
    expect(screen.queryByText("Ani Wijaya")).toBeNull();
    expect(api.fetchLetters).toHaveBeenCalledWith({ employeeId: "e1", includeRevoked: false });
  });
});
