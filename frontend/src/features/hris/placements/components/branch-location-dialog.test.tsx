import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreBranch } from "../types";
import { BranchLocationDialog } from "./branch-location-dialog";

const mutate = vi.fn();
const toastError = vi.fn();
vi.mock("../mutations", () => ({ useSaveBranchLocation: () => ({ mutate, isPending: false }) }));
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("@/components/maps/location-picker-field", () => ({
  LocationPickerField: ({
    onPick,
    radiusM,
  }: {
    onPick: (p: { lat: number; lng: number }) => void;
    radiusM?: number | null;
  }) => (
    <button type="button" data-radius={radiusM} onClick={() => onPick({ lat: -6.8889, lng: 107.596 })}>
      pilih titik
    </button>
  ),
}));

const branches: StoreBranch[] = [
  { id: "b1", code: "PVJ", name: "Paddy PVJ", company_id: "c1", company_name: "Paddy", lat: null, lng: null, geofence_radius_m: null },
  { id: "b2", code: "KPO", name: "Paddy Kopo", company_id: "c1", company_name: "Paddy", lat: "-6.946000", lng: "107.592000", geofence_radius_m: 200 },
];

afterEach(() => {
  cleanup();
  mutate.mockReset();
  toastError.mockReset();
});

describe("BranchLocationDialog", () => {
  it("menghitung toko tanpa koordinat dan menyimpan titik dari peta", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    expect(screen.getByText("1 dari 2 toko belum punya koordinat")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Paddy PVJ/ }));
    fireEvent.click(screen.getByRole("button", { name: "pilih titik" }));
    expect((screen.getByLabelText("Latitude") as HTMLInputElement).value).toBe("-6.8889");
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    expect(mutate).toHaveBeenCalledWith(
      { id: "b1", payload: { lat: -6.8889, lng: 107.596, geofence_radius_m: null } },
      expect.anything()
    );
  });

  it("menempel koordinat Google Maps dan menolak radius di luar batas", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    expect((screen.getByLabelText("Radius (meter)") as HTMLInputElement).value).toBe("200");
    fireEvent.change(screen.getByLabelText("Tempel dari Google Maps"), { target: { value: "-6.9, 107.61" } });
    expect((screen.getByLabelText("Longitude") as HTMLInputElement).value).toBe("107.61");
    fireEvent.change(screen.getByLabelText("Radius (meter)"), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    expect(mutate).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("Radius absensi 25–1000 meter");
  });

  it("lingkaran peta mengikuti radius yang diketik, bawaan 100 m bila kosong", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    expect(screen.getByRole("button", { name: "pilih titik" }).getAttribute("data-radius")).toBe("200");
    fireEvent.change(screen.getByLabelText("Radius (meter)"), { target: { value: "350" } });
    expect(screen.getByRole("button", { name: "pilih titik" }).getAttribute("data-radius")).toBe("350");
    fireEvent.change(screen.getByLabelText("Radius (meter)"), { target: { value: "" } });
    expect(screen.getByRole("button", { name: "pilih titik" }).getAttribute("data-radius")).toBe("100");
  });

  it("selalu mengirim ketiga kunci: koordinat dikosongkan menjadi null, radius bawaan null", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    fireEvent.change(screen.getByLabelText("Latitude"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Longitude"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Radius (meter)"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    expect(mutate).toHaveBeenCalledWith(
      { id: "b2", payload: { lat: null, lng: null, geofence_radius_m: null } },
      expect.anything()
    );
  });

  it("menolak hanya salah satu koordinat dan menampilkan pesan server saat gagal", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    fireEvent.change(screen.getByLabelText("Longitude"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    expect(mutate).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("Latitude dan longitude wajib diisi berdua atau dikosongkan berdua");

    fireEvent.change(screen.getByLabelText("Longitude"), { target: { value: "107.59" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    const opts = mutate.mock.calls[0][1] as { onError: (e: Error) => void };
    opts.onError(new Error("Toko tidak ditemukan"));
    expect(toastError).toHaveBeenLastCalledWith("Toko tidak ditemukan");
  });
});
