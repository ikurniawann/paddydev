import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const refresh = vi.fn();
const upload = vi.fn();
const remove = vi.fn();
const toastSuccess = vi.fn();
const toastError = vi.fn();
const invalidate = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
vi.mock("sonner", () => ({ toast: { success: (m: string) => toastSuccess(m), error: (m: string) => toastError(m) } }));
vi.mock("@/lib/profile-photo", async (orig) => ({
  ...(await orig<typeof import("@/lib/profile-photo")>()),
  uploadProfilePhoto: (d: string) => upload(d),
  deleteProfilePhoto: () => remove(),
}));

import { ProfilePhotoDialog } from "./profile-photo-dialog";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function pick(file: File) {
  fireEvent.change(screen.getByLabelText("Pilih foto"), { target: { files: [file] } });
}

describe("ProfilePhotoDialog (EPIC-087)", () => {
  it("pilih foto → pratinjau → simpan → refresh + toast", async () => {
    upload.mockResolvedValue("/api/files/profile-photos/u/new.jpg");
    render(<ProfilePhotoDialog open onOpenChange={() => {}} name="Paddy HR" currentUrl={null} />);
    expect(screen.queryByRole("button", { name: "Hapus foto" })).toBeNull();
    pick(new File(["x"], "foto.png", { type: "image/png" }));
    await waitFor(() => expect(screen.getByAltText("Pratinjau foto profil")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1));
    expect(upload.mock.calls[0][0]).toMatch(/^data:image\/png;base64,/);
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(toastSuccess).toHaveBeenCalledWith("Foto profil diperbarui");
    // Beranda ESS (React Query) ikut memuat ulang foto.
    expect(invalidate).toHaveBeenCalled();
  });

  it("pratinjau dari pembacaan yang selesai setelah dialog ditutup tidak muncul lagi", async () => {
    const { rerender } = render(<ProfilePhotoDialog open onOpenChange={() => {}} name="Paddy HR" currentUrl={null} />);
    pick(new File(["x"], "foto.png", { type: "image/png" }));
    rerender(<ProfilePhotoDialog open={false} onOpenChange={() => {}} name="Paddy HR" currentUrl={null} />);
    rerender(<ProfilePhotoDialog open onOpenChange={() => {}} name="Paddy HR" currentUrl={null} />);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByAltText("Pratinjau foto profil")).toBeNull();
  });

  it("file salah ditolak di klien", () => {
    render(<ProfilePhotoDialog open onOpenChange={() => {}} name="Paddy HR" currentUrl={null} />);
    pick(new File(["x"], "anim.gif", { type: "image/gif" }));
    expect(toastError).toHaveBeenCalledWith("Foto harus berupa gambar JPG, PNG, atau WebP");
    expect(upload).not.toHaveBeenCalled();
  });

  it("hapus foto bila sudah ada foto", async () => {
    remove.mockResolvedValue(undefined);
    render(<ProfilePhotoDialog open onOpenChange={() => {}} name="Paddy HR" currentUrl="/api/files/profile-photos/u/a.jpg" />);
    fireEvent.click(screen.getByRole("button", { name: "Hapus foto" }));
    await waitFor(() => expect(remove).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith("Foto profil dihapus"));
    expect(refresh).toHaveBeenCalled();
  });
});
