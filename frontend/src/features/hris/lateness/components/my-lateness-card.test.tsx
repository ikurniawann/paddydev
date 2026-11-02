import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MyLateness } from "../types";
import { MyLatenessCard } from "./my-lateness-card";
import { lateDay, myLateness } from "./test-fixtures";

const hook = vi.hoisted(() => ({
  calls: [] as number[],
  data: undefined as MyLateness | undefined,
  /** Data periode back ≥ 1 (undefined + error = periode lain gagal dimuat). */
  other: undefined as { data?: MyLateness; isError?: boolean; isPlaceholderData?: boolean } | undefined,
}));
vi.mock("../queries", () => ({
  useMyLateness: (back: number) => {
    hook.calls.push(back);
    const base = { data: hook.data, isLoading: false, isError: false, isPlaceholderData: false, refetch: vi.fn() };
    return back > 0 && hook.other ? { ...base, ...hook.other } : base;
  },
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children, onValueChange }: { children: React.ReactNode; onValueChange: (v: string) => void }) => (
    <div>
      <button type="button" onClick={() => onValueChange("1")}>
        pilih September
      </button>
      {children}
    </div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

afterEach(() => {
  cleanup();
  hook.calls.length = 0;
  hook.data = undefined;
  hook.other = undefined;
});

describe("MyLatenessCard (EPIC-077 R8)", () => {
  it("3 kali terlambat: badge SP 1, petunjuk tingkat berikutnya, hari terlambat, catatan HR", () => {
    hook.data = myLateness("sp1", [lateDay("2026-10-02"), lateDay("2026-10-05", 20), lateDay("2026-10-09", 45)], {
      code: "sp2", label: "Surat Peringatan 2", at: 5, remaining: 2,
    });
    render(<MyLatenessCard />);
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("kali terlambat")).toBeTruthy();
    expect(screen.getByText("Surat Peringatan 1")).toBeTruthy();
    expect(screen.getByText("2 kali terlambat lagi → Surat Peringatan 2")).toBeTruthy();
    expect(screen.getByText("Hari terlambat (3)")).toBeTruthy();
    expect(screen.getByText("+45 mnt")).toBeTruthy();
    expect(screen.getByText("Total terlambat 1 jam 17 mnt")).toBeTruthy();
    expect(screen.getByText(/Surat peringatan resmi diterbitkan oleh HR/)).toBeTruthy();
    expect(screen.queryByText(/potong/i)).toBeNull();
  });

  it("0 kali: status tidak ada keterlambatan, tanpa daftar hari", () => {
    hook.data = myLateness("none", [], { code: "teguran", label: "Teguran lisan dan tertulis", at: 1, remaining: 1 });
    render(<MyLatenessCard />);
    expect(screen.getByText("Tidak ada keterlambatan")).toBeTruthy();
    expect(screen.getByText(/Tidak ada keterlambatan di periode ini/)).toBeTruthy();
    expect(screen.queryByText(/Hari terlambat/)).toBeNull();
    // Yang tepat waktu tidak diberi petunjuk "1 kali terlambat lagi → …".
    expect(screen.queryByText(/kali terlambat lagi/)).toBeNull();
  });

  it("tanpa jadwal shift: catatan belum dinilai, kecuali sudah ada hari terlambat", () => {
    hook.data = myLateness("none", []);
    const { rerender } = render(<MyLatenessCard hasSchedule={false} />);
    expect(screen.getByText(/Jadwal shift belum diatur HR/)).toBeTruthy();
    hook.data = myLateness("teguran", [lateDay("2026-10-02")]);
    rerender(<MyLatenessCard hasSchedule={false} />);
    expect(screen.queryByText(/Jadwal shift belum diatur HR/)).toBeNull();
  });

  it("ketentuan perusahaan bisa dibuka", () => {
    hook.data = myLateness("none", []);
    render(<MyLatenessCard />);
    const toggle = screen.getByRole("button", { name: /Jam Kerja & Ketentuan Keterlambatan/ });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText(/klik “check-in” saat jam masuk/)).toBeTruthy();
    expect(screen.getByText(/surat peringatan 3 dan\/atau Pemutusan Hubungan Kerja/)).toBeTruthy();
    // EPIC-078: jam kerja HQ dan Operations tampil apa adanya.
    const hq = screen.getByRole("region", { name: "Karyawan Headquarter" });
    expect(hq.textContent).toContain("08.00 – 09.00 WIB");
    expect(hq.textContent).toContain("17.00 – 18.00 WIB");
    const ops = screen.getByRole("region", { name: "Karyawan Operations" });
    expect(ops.textContent).toContain("Seluruh staff Warehouse.");
    expect(ops.textContent).toMatch(/Sabtu\s*08\.00 – 14\.00 WIB/);
    // EPIC-079: sistem shift (Shift 1–3) dan aturan hari libur tampil apa adanya.
    const shift = screen.getByRole("region", { name: "Karyawan Sistem Shift" });
    expect(shift.textContent).toContain("Retail Operasional, Host Live, CRM dan Produksi.");
    expect(shift.textContent).toContain("Senin – Minggu (libur 1 hari setiap minggu)");
    expect(shift.textContent).toMatch(/Shift 1\s*08\.00 – 16\.00 WIB/);
    expect(shift.textContent).toMatch(/Shift 2\s*11\.00 – 19\.00 WIB/);
    expect(shift.textContent).toContain("13.30 – 21.30 WIB");
    expect(screen.getByText(/tidak mengikuti hari libur nasional/)).toBeTruthy();
    expect(screen.getByText(/Hari cuti bersama tidak secara otomatis menjadi hari libur/)).toBeTruthy();
    expect(screen.getByText(/Karyawan wajib menaati ketentuan jam kerja/)).toBeTruthy();
    expect(screen.getByText(/jam mulai dan berakhirnya shift dapat berbeda/)).toBeTruthy();
  });

  it("ganti periode meminta back = 1", () => {
    hook.data = myLateness("none", []);
    render(<MyLatenessCard />);
    fireEvent.click(screen.getByRole("button", { name: "pilih September" }));
    expect(hook.calls).toContain(1);
  });

  it("periode lain gagal dimuat: pesan galat, pemilih periode tetap ada", () => {
    hook.data = myLateness("none", []);
    hook.other = { data: undefined, isError: true };
    render(<MyLatenessCard />);
    fireEvent.click(screen.getByRole("button", { name: "pilih September" }));
    expect(screen.getByText("Info keterlambatan gagal dimuat.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "pilih September" })).toBeTruthy();
  });

  it("selama memuat periode lain: isi diredupkan dan ditandai sibuk", () => {
    hook.data = myLateness("none", []);
    hook.other = { data: myLateness("none", []), isPlaceholderData: true };
    const { container } = render(<MyLatenessCard />);
    fireEvent.click(screen.getByRole("button", { name: "pilih September" }));
    expect(screen.getByText("Memuat periode…")).toBeTruthy();
    expect(container.querySelector('[aria-busy="true"]')).toBeTruthy();
  });
});
