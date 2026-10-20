// id dikanonikkan sebagai UUID sebelum guard (sama dengan Go validate.CanonicalUUID, temuan EPIC-061).
import { describe, expect, it } from "vitest";
import { canonicalUuid } from "./canonical-uuid";

const ID = "7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3";

describe("canonicalUuid", () => {
  it.each([ID, ID.toUpperCase(), ID.replaceAll("-", ""), `{${ID}}`, `{${ID.replaceAll("-", "").toUpperCase()}}`])(
    "%s → bentuk kanonik",
    (input) => expect(canonicalUuid(input)).toBe(ID)
  );

  it.each(["", "nope", ID.slice(0, -1), `${ID.slice(0, -1)}z`, "7b0c1d2e3f40-4a5b-8c6d-7e8f90a1b2c3", ` ${ID}`, `{${ID}`])(
    "%j → null",
    (input) => expect(canonicalUuid(input)).toBeNull()
  );
});
