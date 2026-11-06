import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { IssueLetterDialog } from "./issue-letter-dialog";
import { letter, renderWithClient } from "./test-setup";

const api = vi.hoisted(() => ({
  fetchLetters: vi.fn(),
  fetchMyLetters: vi.fn(),
  fetchLetterLateness: vi.fn(),
  issueLetter: vi.fn(),
  revokeLetter: vi.fn(),
  uploadLetterAttachment: vi.fn(),
}));
const placementsApi = vi.hoisted(() => ({ searchPlacements: vi.fn() }));
vi.mock("../api", async (importOriginal) => ({ ...(await importOriginal<typeof import("../api")>()), ...api }));
vi.mock("@/features/hris/placements/api", () => placementsApi);
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayWib: () => "2026-11-06",
}));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({ options, value, onChange, placeholder }: {
    options: { value: string; label: string }[];
    value?: string;
    onChange: (v: string) => void;
    placeholder?: string;
  }) => (
    <select aria-label={placeholder} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));

beforeEach(() => {
  Object.values(api).forEach((fn) => fn.mockReset());
  placementsApi.searchPlacements.mockResolvedValue({
    rows: [{ employee_id: "e1", nip: "1001", full_name: "Ani Wijaya", position_title: "Shopkeeper", placements: [] }],
    total: 1, page: 1, page_size: 25, counts: { all: 1, unplaced: 0, no_schedule: 0, multi: 0 },
  });
  api.fetchLetterLateness.mockResolvedValue({ count: 3, level: "sp1", label: "Surat Peringatan 1", period_start: "2026-10-21", period_end: "2026-11-20" });
  api.uploadLetterAttachment.mockResolvedValue("disciplinary-letters/e1/1-abcdef0123456789.pdf");
  api.issueLetter.mockResolvedValue(letter());
});
afterEach(cleanup);

describe("IssueLetterDialog (EPIC-094)", () => {
  it("pilih karyawan → info keterlambatan; terbitkan dengan lampiran", async () => {
    const onOpenChange = vi.fn();
    renderWithClient(<IssueLetterDialog open onOpenChange={onOpenChange} />);
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });
    expect(await screen.findByText("Terlambat 3 kali di periode ini (ketentuan: Surat Peringatan 1)")).toBeTruthy();
    expect(api.fetchLetterLateness).toHaveBeenCalledWith("e1");

    fireEvent.change(screen.getByLabelText("Jenis surat"), { target: { value: "sp1" } });
    fireEvent.change(screen.getByLabelText("Tanggal terbit"), { target: { value: "2026-11-05" } });
    fireEvent.change(screen.getByLabelText("Catatan"), { target: { value: "Terlambat 3 kali" } });
    const file = new File(["%PDF-1.4"], "sp1.pdf", { type: "application/pdf" });
    fireEvent.change(screen.getByLabelText("Lampiran (opsional)"), { target: { files: [file] } });
    await waitFor(() => expect(api.uploadLetterAttachment).toHaveBeenCalledWith(file, "e1"));
    expect(await screen.findByText("sp1.pdf")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Terbitkan" }));
    await waitFor(() => expect(api.issueLetter).toHaveBeenCalled());
    expect(api.issueLetter).toHaveBeenCalledWith({
      employee_id: "e1",
      letter_type: "sp1",
      issued_date: "2026-11-05",
      notes: "Terlambat 3 kali",
      attachment_path: "disciplinary-letters/e1/1-abcdef0123456789.pdf",
    });
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Surat Peringatan 1 diterbitkan"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("tanggal terbit di masa depan: tombol nonaktif dengan pesan", async () => {
    renderWithClient(<IssueLetterDialog open onOpenChange={vi.fn()} employeeId="e1" employeeName="Ani Wijaya" />);
    fireEvent.change(screen.getByLabelText("Tanggal terbit"), { target: { value: "2026-11-07" } });
    expect(screen.getByText("Tanggal terbit tidak boleh di masa depan")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Terbitkan" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("dari profil: karyawan terkunci tanpa pemilih", async () => {
    renderWithClient(<IssueLetterDialog open onOpenChange={vi.fn()} employeeId="e1" employeeName="Ani Wijaya" />);
    expect(screen.getByText("Ani Wijaya")).toBeTruthy();
    expect(screen.queryByLabelText("Pilih karyawan")).toBeNull();
    expect(await screen.findByText(/Terlambat 3 kali di periode ini/)).toBeTruthy();
  });
});
