import { afterEach, describe, expect, it, vi } from "vitest";
import { deleteProfilePhoto, uploadProfilePhoto, validateProfilePhoto } from "./profile-photo";

afterEach(() => vi.unstubAllGlobals());

describe("validateProfilePhoto (EPIC-087)", () => {
  it("menolak format selain JPG/PNG/WebP", () => {
    expect(validateProfilePhoto({ type: "image/gif", size: 1000 })).toBe("Foto harus berupa gambar JPG, PNG, atau WebP");
  });
  it("menolak lebih dari 5 MB", () => {
    expect(validateProfilePhoto({ type: "image/png", size: 6 * 1024 * 1024 })).toBe("Ukuran foto maksimal 5 MB");
  });
  it("menerima JPEG 1 MB", () => {
    expect(validateProfilePhoto({ type: "image/jpeg", size: 1024 * 1024 })).toBeNull();
  });
});

describe("upload/delete", () => {
  it("unggah mengembalikan avatar_url, error memakai pesan server", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { avatar_url: "/api/files/profile-photos/u/x.jpg" } }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "Ukuran foto maksimal 5 MB" }), { status: 400 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(uploadProfilePhoto("data:image/png;base64,AAA")).resolves.toBe("/api/files/profile-photos/u/x.jpg");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/auth/me/avatar");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ image: "data:image/png;base64,AAA" });
    await expect(uploadProfilePhoto("data:image/png;base64,AAA")).rejects.toThrow("Ukuran foto maksimal 5 MB");
  });
  it("hapus memanggil DELETE", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { avatar_url: null } }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await deleteProfilePhoto();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/auth/me/avatar");
    expect(fetchMock.mock.calls[0][1].method).toBe("DELETE");
  });
});
