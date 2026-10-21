import { expect, it } from "vitest";
import { ACTION_LABELS, DESTRUCTIVE_ACTIONS } from "./audit-labels";

it("employee.rehire berlabel Indonesia dan bukan aksi destruktif", () => {
  expect(ACTION_LABELS["employee.rehire"]).toBe("Karyawan diaktifkan kembali");
  expect(DESTRUCTIVE_ACTIONS.has("employee.rehire")).toBe(false);
});
