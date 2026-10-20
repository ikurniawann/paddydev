import { ApiError } from "@/lib/api/auth";

/**
 * Bentuk kanonik (huruf kecil, bertanda hubung) dari id yang diterima kolom uuid: huruf apa pun,
 * dengan/tanpa empat tanda hubung, boleh dalam kurung kurawal. Null bila bukan UUID. Guard
 * membandingkan id dalam bentuk ini, jadi setiap id dari request dikanonikkan sekali di input
 * (sama dengan Go validate.CanonicalUUID, temuan re-review EPIC-061).
 */
export function canonicalUuid(input: string): string | null {
  let s = input;
  if (s.startsWith("{") || s.endsWith("}")) {
    if (s.length < 2 || !s.startsWith("{") || !s.endsWith("}")) return null;
    s = s.slice(1, -1);
  }
  if (s.length === 36) {
    if (s[8] !== "-" || s[13] !== "-" || s[18] !== "-" || s[23] !== "-") return null;
    s = s.replaceAll("-", "");
  }
  if (!/^[0-9a-fA-F]{32}$/.test(s)) return null;
  const h = s.toLowerCase();
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

/** Id path yang dikanonikkan; 400 "Invalid id" bila bukan UUID. */
export function requireCanonicalId(input: string): string {
  const id = canonicalUuid(input);
  if (!id) throw ApiError.badRequest("Invalid id");
  return id;
}
