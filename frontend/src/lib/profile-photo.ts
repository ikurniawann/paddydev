/** Foto profil akun sendiri (EPIC-087): validasi klien + panggilan API Go. */

export const PROFILE_PHOTO_MAX_BYTES = 5 * 1024 * 1024;
const TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);

export function validateProfilePhoto(file: { type: string; size: number }): string | null {
  if (!TYPES.has(file.type)) return "Foto harus berupa gambar JPG, PNG, atau WebP";
  if (file.size > PROFILE_PHOTO_MAX_BYTES) return "Ukuran foto maksimal 5 MB";
  return null;
}

async function call(init: RequestInit): Promise<string | null> {
  const res = await fetch("/api/auth/me/avatar", init);
  const json = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(json.error ?? "Gagal menyimpan foto profil");
  return json.data?.avatar_url ?? null;
}

export async function uploadProfilePhoto(dataUrl: string): Promise<string> {
  const url = await call({
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ image: dataUrl }),
  });
  return url ?? "";
}

export async function deleteProfilePhoto(): Promise<void> {
  await call({ method: "DELETE" });
}
