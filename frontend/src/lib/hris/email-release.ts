/** Siapa pemilik email saat menambah karyawan (EPIC-056 §1.2). */
export interface EmailOwner {
  id: string;
  nip: string;
  full_name: string;
  is_active: boolean;
  end_date: string | null;
}

export type EmailCheck =
  | { kind: "free" }
  | { kind: "taken"; message: string }
  | { kind: "release"; message: string };

const dmy = (iso: string) => `${iso.slice(8, 10)}/${iso.slice(5, 7)}/${iso.slice(0, 4)}`;

/** Aktif = tolak; resign = tawarkan lepas email ke data baru. */
export function emailCheck(owner: EmailOwner | null): EmailCheck {
  if (!owner) return { kind: "free" };
  if (owner.is_active) {
    return { kind: "taken", message: `Email sudah dipakai ${owner.full_name} (NIP ${owner.nip})` };
  }
  const since = owner.end_date ? `, resign ${dmy(owner.end_date)}` : ", resign";
  return {
    kind: "release",
    message: `Email ini milik ${owner.full_name} (NIP ${owner.nip}${since}). Lepas email dari data lama dan buat karyawan baru?`,
  };
}
