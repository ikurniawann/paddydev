import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GEOFENCE_DEFAULT_RADIUS_M } from "@/lib/hris/geofence";
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

  it("memakai radius bawaan bila radiusM tidak diberikan", async () => {
    const { rerender } = render(<LocationPickerField lat="-6.9" lng="107.6" onPick={() => {}} />);
    expect((await screen.findByTestId("map-radius")).textContent).toBe(String(GEOFENCE_DEFAULT_RADIUS_M));
    rerender(<LocationPickerField lat="-6.9" lng="107.6" radiusM={300} onPick={() => {}} />);
    expect(screen.getByTestId("map-radius").textContent).toBe("300");
  });

  it("meneruskan klik peta ke onPick", async () => {
    const onPick = vi.fn();
    render(<LocationPickerField lat="" lng="" onPick={onPick} />);
    fireEvent.click(await screen.findByText("klik-peta"));
    expect(onPick).toHaveBeenCalledWith({ lat: -6.9, lng: 107.6 });
  });
});
