import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreBranch } from "../types";
import { BranchLocationDialog } from "./branch-location-dialog";

const mutate = vi.fn();
const toastError = vi.fn();
const mutateHours = vi.fn();
vi.mock("../mutations", () => ({
  useSaveBranchLocation: () => ({ mutate, isPending: false }),
  useSaveBranchMinWorkHours: () => ({ mutate: mutateHours, isPending: false }),
}));
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("@/components/maps/location-picker-field", () => ({
  LocationPickerField: ({
    onPick,
    radiusM,
    onSearch,
  }: {
    onPick: (p: { lat: number; lng: number }) => void;
    radiusM?: number | null;
    onSearch?: unknown;
  }) => (
    <button
      type="button"
      data-radius={radiusM}
      data-searchable={String(typeof onSearch === "function")}
      onClick={() => onPick({ lat: -6.8889, lng: 107.596 })}
    >
      pilih titik
    </button>
  ),
}));

const branches: StoreBranch[] = [
  { id: "b1", code: "PVJ", name: "Paddy PVJ", company_id: "c1", company_name: "Paddy", lat: null, lng: null, geofence_radius_m: null },
  { id: "b2", code: "KPO", name: "Paddy Kopo", company_id: "c1", company_name: "Paddy", lat: "-6.946000", lng: "107.592000", geofence_radius_m: 200, min_work_hours: "9.00" },
];

afterEach(() => {
  cleanup();
  mutate.mockReset();
  mutateHours.mockReset();
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

  it("menempel link Google Maps lengkap; link pendek dan teks lain diberi petunjuk", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy PVJ/ }));
    expect(screen.getByRole("button", { name: "pilih titik" }).getAttribute("data-searchable")).toBe("true");
    const paste = screen.getByLabelText("Tempel dari Google Maps");
    fireEvent.change(paste, {
      target: { value: "https://www.google.com/maps/place/Paddy/@-6.88,107.61,17z/data=!3m1!8m2!3d-6.8889123!4d107.6131456" },
    });
    expect((screen.getByLabelText("Latitude") as HTMLInputElement).value).toBe("-6.888912");
    expect((screen.getByLabelText("Longitude") as HTMLInputElement).value).toBe("107.613146");
    fireEvent.change(paste, { target: { value: "https://maps.app.goo.gl/AbCdEf123" } });
    expect(screen.getByText(/Link pendek tidak memuat koordinat/)).toBeTruthy();
    fireEvent.change(paste, { target: { value: "Jl. Dago 1" } });
    expect(screen.getByText(/Tidak terbaca sebagai koordinat/)).toBeTruthy();
    expect((screen.getByLabelText("Latitude") as HTMLInputElement).value).toBe("-6.888912");
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

  it("menyimpan jam kerja minimum dan menolak di luar 1–24 jam (AB-05)", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy PVJ/ }));
    expect((screen.getByLabelText("Jam kerja minimum (jam)") as HTMLInputElement).value).toBe("");
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "0.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("Jam kerja minimum 1–24 jam (kelipatan 0,25)");
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "9" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).toHaveBeenCalledWith({ id: "b1", payload: { min_work_hours: 9 } }, expect.anything());
  });

  it("memuat jam minimum tersimpan dan mengosongkannya menjadi null", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    expect((screen.getByLabelText("Jam kerja minimum (jam)") as HTMLInputElement).value).toBe("9");
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).toHaveBeenCalledWith({ id: "b2", payload: { min_work_hours: null } }, expect.anything());
  });

  it("koma desimal diterima; teks bukan angka ditolak, bukan diam-diam dikosongkan", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy PVJ/ }));
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "8,5" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).toHaveBeenCalledWith({ id: "b1", payload: { min_work_hours: 8.5 } }, expect.anything());
    mutateHours.mockReset();
    for (const bad of ["abc", "1e1", "-9", "9,", " 9 x"]) {
      fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: bad } });
      fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    }
    expect(mutateHours).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledTimes(5);
  });

  it("kegagalan simpan jam minimum ditampilkan dengan pesan server", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    const opts = mutateHours.mock.calls[0][1] as { onError: (e: Error) => void };
    opts.onError(new Error("Toko tidak ditemukan"));
    expect(toastError).toHaveBeenLastCalledWith("Toko tidak ditemukan");
  });
});
