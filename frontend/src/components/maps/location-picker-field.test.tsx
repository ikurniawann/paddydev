import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LatLngPoint } from "@/lib/maps/coords";

// OpenLayers tidak berjalan di jsdom: peta diganti stub yang memperlihatkan props-nya.
vi.mock("./location-picker-map", () => ({
  LocationPickerMap: ({
    value,
    radiusM,
    onPick,
  }: {
    value: LatLngPoint | null;
    radiusM?: number | null;
    onPick: (p: LatLngPoint) => void;
  }) => (
    <div>
      <div data-testid="map-value">{value ? `${value.lat},${value.lng}` : "none"}</div>
      <div data-testid="map-radius">{String(radiusM)}</div>
      <button type="button" onClick={() => onPick({ lat: -6.9, lng: 107.6 })}>
        klik-peta
      </button>
    </div>
  ),
}));

import { LocationPickerField } from "./location-picker-field";

afterEach(cleanup);

describe("LocationPickerField", () => {
  it("meneruskan titik dari input teks ke peta, kosong bila tidak valid", async () => {
    const { rerender } = render(<LocationPickerField lat="-6.9" lng="107.6" onPick={() => {}} />);
    expect((await screen.findByTestId("map-value")).textContent).toBe("-6.9,107.6");
    rerender(<LocationPickerField lat="" lng="107.6" onPick={() => {}} />);
    expect(screen.getByTestId("map-value").textContent).toBe("none");
  });

  it("meneruskan radiusM apa adanya (fallback bawaan ada di peta)", async () => {
    const { rerender } = render(<LocationPickerField lat="-6.9" lng="107.6" onPick={() => {}} />);
    expect((await screen.findByTestId("map-radius")).textContent).toBe("undefined");
    rerender(<LocationPickerField lat="-6.9" lng="107.6" radiusM={300} onPick={() => {}} />);
    expect(screen.getByTestId("map-radius").textContent).toBe("300");
  });

  it("menaruh catatan tambahan di baris bantuan yang sama", async () => {
    render(<LocationPickerField lat="" lng="" onPick={() => {}} note="Catatan khusus." />);
    await screen.findByTestId("map-value");
    expect(screen.getAllByText(/Catatan khusus\./)).toHaveLength(1);
    expect(screen.getByText(/Klik peta untuk menaruh titik toko\..*Catatan khusus\./)).toBeTruthy();
  });

  it("meneruskan klik peta ke onPick", async () => {
    const onPick = vi.fn();
    render(<LocationPickerField lat="" lng="" onPick={onPick} />);
    fireEvent.click(await screen.findByText("klik-peta"));
    expect(onPick).toHaveBeenCalledWith({ lat: -6.9, lng: 107.6 });
  });

  it("Pakai lokasi saya mengisi titik dari GPS dan menampilkan akurasi", async () => {
    const getCurrentPosition = vi.fn((ok: PositionCallback) =>
      ok({ coords: { latitude: -6.88891234, longitude: 107.61314567, accuracy: 12.4 } } as GeolocationPosition)
    );
    vi.stubGlobal("navigator", { ...navigator, geolocation: { getCurrentPosition } });
    const onPick = vi.fn();
    render(<LocationPickerField lat="" lng="" onPick={onPick} />);
    fireEvent.click(screen.getByRole("button", { name: /Pakai lokasi saya/ }));
    expect(onPick).toHaveBeenCalledWith({ lat: -6.888912, lng: 107.613146 });
    expect(screen.getByText(/akurasi ±12 m/)).toBeTruthy();
    vi.unstubAllGlobals();
  });

  it("Pakai lokasi saya: izin ditolak dan akurasi rendah diberi pesan", async () => {
    const denied = vi.fn((_ok: PositionCallback, fail: PositionErrorCallback) =>
      fail({ code: 1, PERMISSION_DENIED: 1 } as GeolocationPositionError)
    );
    vi.stubGlobal("navigator", { ...navigator, geolocation: { getCurrentPosition: denied } });
    const onPick = vi.fn();
    const { unmount } = render(<LocationPickerField lat="" lng="" onPick={onPick} />);
    fireEvent.click(screen.getByRole("button", { name: /Pakai lokasi saya/ }));
    expect(onPick).not.toHaveBeenCalled();
    expect(screen.getByText(/Izin lokasi ditolak/)).toBeTruthy();
    unmount();

    const rough = vi.fn((ok: PositionCallback) =>
      ok({ coords: { latitude: -6.9, longitude: 107.6, accuracy: 350 } } as GeolocationPosition)
    );
    vi.stubGlobal("navigator", { ...navigator, geolocation: { getCurrentPosition: rough } });
    render(<LocationPickerField lat="" lng="" onPick={onPick} />);
    fireEvent.click(screen.getByRole("button", { name: /Pakai lokasi saya/ }));
    expect(screen.getByText(/Akurasi rendah/)).toBeTruthy();
    vi.unstubAllGlobals();
  });

  it("kotak cari hanya muncul bila onSearch diberikan; hasil dipilih menjadi titik", async () => {
    const { unmount } = render(<LocationPickerField lat="" lng="" onPick={() => {}} />);
    expect(screen.queryByPlaceholderText(/Cari nama tempat/)).toBeNull();
    unmount();

    const onSearch = vi.fn().mockResolvedValue([{ name: "Paddy Dago, Bandung", lat: -6.88891234, lng: 107.6131 }]);
    const onPick = vi.fn();
    render(<LocationPickerField lat="" lng="" onPick={onPick} onSearch={onSearch} />);
    const input = screen.getByPlaceholderText(/Cari nama tempat/);
    fireEvent.change(input, { target: { value: "paddy dago" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onSearch).toHaveBeenCalledWith("paddy dago");
    fireEvent.click(await screen.findByRole("button", { name: /Paddy Dago, Bandung/ }));
    expect(onPick).toHaveBeenCalledWith({ lat: -6.888912, lng: 107.6131 });
    expect(screen.queryByRole("button", { name: /Paddy Dago, Bandung/ })).toBeNull();
  });

  it("kotak cari: kurang dari 3 huruf tidak dicari, kosong dan gagal diberi pesan", async () => {
    const onSearch = vi.fn().mockResolvedValueOnce([]).mockRejectedValueOnce(new Error("Pencarian lokasi sedang tidak tersedia."));
    render(<LocationPickerField lat="" lng="" onPick={() => {}} onSearch={onSearch} />);
    const input = screen.getByPlaceholderText(/Cari nama tempat/);
    fireEvent.change(input, { target: { value: "pd" } });
    fireEvent.click(screen.getByRole("button", { name: "Cari" }));
    expect(onSearch).not.toHaveBeenCalled();
    expect(screen.getByText(/minimal 3 huruf/)).toBeTruthy();

    fireEvent.change(input, { target: { value: "tidak ada" } });
    fireEvent.click(screen.getByRole("button", { name: "Cari" }));
    expect(await screen.findByText(/Tidak ditemukan/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Cari" }));
    await waitFor(() => expect(screen.getByText(/Pencarian lokasi sedang tidak tersedia/)).toBeTruthy());
  });
});
