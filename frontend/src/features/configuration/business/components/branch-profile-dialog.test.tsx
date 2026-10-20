import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LatLngPoint } from "@/lib/maps/coords";
import type { BranchProfile } from "../api";

const { fetchBranchProfile, saveBranchProfile } = vi.hoisted(() => ({
  fetchBranchProfile: vi.fn(),
  saveBranchProfile: vi.fn(),
}));
vi.mock("../api", () => ({ fetchBranchProfile, saveBranchProfile }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// OpenLayers tidak berjalan di jsdom: peta diganti stub.
vi.mock("@/components/maps/location-picker-map", () => ({
  LocationPickerMap: ({ value, onPick }: { value: LatLngPoint | null; onPick: (p: LatLngPoint) => void }) => (
    <div data-testid="map">
      <span data-testid="map-value">{value ? `${value.lat},${value.lng}` : "none"}</span>
      <button type="button" onClick={() => onPick({ lat: -6.8889, lng: 107.596 })}>
        klik-peta
      </button>
    </div>
  ),
}));

import { BranchProfileDialog } from "./branch-profile-dialog";

const profile: BranchProfile = {
  id: "b1",
  name: "Bandung",
  slug: "bandung",
  is_public: true,
  address: null,
  city: null,
  postcode: null,
  lat: -6.9,
  lng: 107.6,
  phone: null,
  email: null,
  instagram: null,
  directions: null,
  hero_image_url: null,
  benefits: [],
  accordions: { facilities: [], parking: [], team: [], community: [] },
  extras: [],
  testimonials: [],
};

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <BranchProfileDialog branchId="b1" branchName="Bandung" onClose={() => {}} />
    </QueryClientProvider>
  );
}

const latInput = () => screen.getByLabelText("Latitude") as HTMLInputElement;
const lngInput = () => screen.getByLabelText("Longitude") as HTMLInputElement;

beforeEach(() => {
  fetchBranchProfile.mockResolvedValue(profile);
  saveBranchProfile.mockImplementation((_id: string, body: object) => Promise.resolve({ ...profile, ...body }));
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("BranchProfileDialog: pemilih titik", () => {
  it("menampilkan peta dengan titik dari lat/lng profil, di luar <label>", async () => {
    renderDialog();
    const map = await screen.findByTestId("map");
    expect(screen.getByTestId("map-value").textContent).toBe("-6.9,107.6");
    expect(map.closest("label")).toBeNull();
    expect(screen.getByText("Titik di peta")).toBeTruthy();
  });

  it("klik peta mengisi input Latitude dan Longitude", async () => {
    renderDialog();
    fireEvent.click(await screen.findByText("klik-peta"));
    expect(latInput().value).toBe("-6.8889");
    expect(lngInput().value).toBe("107.596");
    expect(screen.getByTestId("map-value").textContent).toBe("-6.8889,107.596");
  });

  it("mengetik lat/lng memindahkan titik di peta", async () => {
    renderDialog();
    await screen.findByTestId("map");
    fireEvent.change(latInput(), { target: { value: "-6.5" } });
    expect(screen.getByTestId("map-value").textContent).toBe("-6.5,107.6");
  });

  it("menempel koordinat Google Maps di salah satu input mengisi keduanya", async () => {
    renderDialog();
    await screen.findByTestId("map");
    fireEvent.paste(latInput(), { clipboardData: { getData: () => "-6.2, 106.8" } });
    expect(latInput().value).toBe("-6.2");
    expect(lngInput().value).toBe("106.8");
    fireEvent.paste(lngInput(), { clipboardData: { getData: () => "-6.888900, 107.596000" } });
    expect(latInput().value).toBe("-6.8889");
    expect(lngInput().value).toBe("107.596");
  });

  it("menempel teks bukan koordinat tidak mengubah apa pun", async () => {
    renderDialog();
    await screen.findByTestId("map");
    fireEvent.paste(latInput(), { clipboardData: { getData: () => "-6.5" } });
    expect(latInput().value).toBe("-6.9");
    expect(lngInput().value).toBe("107.6");
  });

  it("menyimpan lat/lng hasil klik peta lewat alur simpan yang sama", async () => {
    renderDialog();
    fireEvent.click(await screen.findByText("klik-peta"));
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(saveBranchProfile).toHaveBeenCalledTimes(1));
    const [id, body] = saveBranchProfile.mock.calls[0];
    expect(id).toBe("b1");
    expect(body).toMatchObject({ lat: -6.8889, lng: 107.596, slug: "bandung", is_public: true });
    expect(body).not.toHaveProperty("geofence_radius_m");
  });
});
