import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TripRequestDialog } from "./trip-request-dialog";

const mutate = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();
vi.mock("../mutations", () => ({ useCreateBusinessTrip: () => ({ mutate, isPending: false }) }));
vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({ options, onChange }: { options: { value: string; label: string }[]; onChange: (v: string) => void }) => (
    <div>
      {options.map((o) => (
        <button key={o.value} type="button" onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  mutate.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
});

function fill(over: Partial<Record<"Tanggal mulai" | "Tanggal selesai" | "Tujuan" | "Keperluan", string>> = {}) {
  const v = { "Tanggal mulai": "2099-03-02", "Tanggal selesai": "2099-03-04", Tujuan: "  Gudang Cikarang ", Keperluan: " Stock opname ", ...over };
  for (const [label, value] of Object.entries(v)) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  }
}

describe("TripRequestDialog", () => {
  it("menolak form tidak valid dengan pesan seperti server", () => {
    render(<TripRequestDialog open onOpenChange={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Tanggal mulai dan selesai wajib diisi");
    fill({ "Tanggal mulai": "2000-01-01", "Tanggal selesai": "2000-01-02" });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Tanggal mulai tidak boleh sebelum hari ini");
    fill({ Tujuan: "ab" });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Tujuan minimal 3 karakter");
    expect(mutate).not.toHaveBeenCalled();
  });

  it("mengirim payload ter-trim tanpa employee_id untuk pengajuan sendiri", () => {
    const onOpenChange = vi.fn();
    mutate.mockImplementation((_p, opts) => opts.onSuccess({ message: "Pengajuan terkirim dari server" }));
    render(<TripRequestDialog open onOpenChange={onOpenChange} />);
    fill();
    expect(screen.getByText(/3 hari/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(mutate).toHaveBeenCalledWith(
      { start_date: "2099-03-02", end_date: "2099-03-04", destination: "Gudang Cikarang", purpose: "Stock opname" },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Pengajuan terkirim dari server");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("menampilkan pesan error server apa adanya (mis. irisan tanggal)", () => {
    mutate.mockImplementation((_p, opts) => opts.onError(new Error("Sudah ada Perjalanan Dinas menunggu/disetujui yang beririsan dengan tanggal ini")));
    const onOpenChange = vi.fn();
    render(<TripRequestDialog open onOpenChange={onOpenChange} />);
    fill();
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Sudah ada Perjalanan Dinas menunggu/disetujui yang beririsan dengan tanggal ini");
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("mengosongkan isian saat dialog ditutup", () => {
    const { rerender } = render(<TripRequestDialog open onOpenChange={() => {}} />);
    fill();
    fireEvent.click(screen.getByRole("button", { name: "Batal" }));
    rerender(<TripRequestDialog open onOpenChange={() => {}} />);
    expect((screen.getByLabelText("Tujuan") as HTMLInputElement).value).toBe("");
  });

  it("mode HRD: wajib pilih karyawan lalu mengirim employee_id", () => {
    render(<TripRequestDialog open onOpenChange={() => {}} employees={[{ value: "e1", label: "Budi (N01)" }]} />);
    fill();
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(toastError).toHaveBeenCalledWith("Pilih karyawan");
    expect(mutate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Budi (N01)" }));
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(mutate).toHaveBeenCalledWith(
      { start_date: "2099-03-02", end_date: "2099-03-04", destination: "Gudang Cikarang", purpose: "Stock opname", employee_id: "e1" },
      expect.anything()
    );
  });
});
