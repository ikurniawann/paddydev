import { expect, it } from "vitest";
import { emailCheck } from "./email-release";

it("email bebas, milik karyawan aktif, atau milik karyawan resign", () => {
  expect(emailCheck(null)).toEqual({ kind: "free" });
  expect(emailCheck({ id: "1", nip: "202310037", full_name: "Prananda", is_active: true, end_date: null })).toEqual({
    kind: "taken",
    message: "Email sudah dipakai Prananda (NIP 202310037)",
  });
  expect(emailCheck({ id: "2", nip: "201901001", full_name: "Rina", is_active: false, end_date: "2025-03-31" })).toEqual({
    kind: "release",
    message: "Email ini milik Rina (NIP 201901001, resign 31/03/2025). Lepas email dari data lama dan buat karyawan baru?",
  });
});
