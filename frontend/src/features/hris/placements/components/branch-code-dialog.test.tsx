import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { BranchCodeImportPreview } from "../types";
import { BranchCodeDialog } from "./branch-code-dialog";

const api = vi.hoisted(() => ({
  importBranchCodeFile: vi.fn(),
  downloadBranchCodeErrors: vi.fn(),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("../api", () => ({ ...api, branchCodeTemplateUrl: "/api/hris/branches/template" }));
vi.mock("sonner", () => ({ toast }));
afterEach(cleanup);
beforeEach(() => {
  api.importBranchCodeFile.mockReset();
  api.downloadBranchCodeErrors.mockReset();
  toast.success.mockReset();
  toast.error.mockReset();
});

const change = {
  line: 4,
  old_code: "ODOO-PPGA",
  new_code: "PPGA",
  old_name: "PPGA - Paddy Playstore Backyard",
  new_name: "PPGA - Paddy Playstore Backyard",
  old_active: true,
  new_active: false,
};
const preview = (over: Partial<BranchCodeImportPreview> = {}) => ({
  data: {
    summary: { rows: 56, changed: 1, code: 1, name: 0, deactivate: 1, activate: 0 },
    errors: [],
    warnings: [],
    fingerprint: "fp1",
    changes: [change],
    ...over,
  },
});

function setup() {
  const qc = new QueryClient();
  const spy = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <BranchCodeDialog open onOpenChange={() => {}} />
    </QueryClientProvider>
  );
  return { spy };
}

async function choose() {
  const file = new File(["x"], "toko.xlsx");
  fireEvent.change(screen.getByLabelText("File kode & status toko (.xlsx)"), { target: { files: [file] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
  return file;
}

it("menautkan template dan menahan simpan selama file masih punya error", async () => {
  api.importBranchCodeFile.mockResolvedValue(
    preview({ errors: [{ line: 5, column: "Kode Baru", name: "", message: "Kode TWEM sudah dipakai toko lain" }] })
  );
  setup();
  expect(screen.getByRole("link", { name: /Unduh template/ }).getAttribute("href")).toBe("/api/hris/branches/template");
  await choose();
  expect(await screen.findByText("Kode TWEM sudah dipakai toko lain")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Simpan perubahan toko" }) as HTMLButtonElement).disabled).toBe(true);
});

it("menampilkan perubahan, butuh centang peringatan, lalu menyimpan", async () => {
  api.importBranchCodeFile.mockResolvedValueOnce(
    preview({ warnings: [{ line: 4, column: "", name: "", message: "Kode ODOO-PPGA → PPGA: unduh ulang" }] })
  );
  api.importBranchCodeFile.mockResolvedValueOnce({ data: { summary: {} }, message: "1 toko diperbarui" });
  const { spy } = setup();
  const file = await choose();
  expect(await screen.findByText("ODOO-PPGA → PPGA")).toBeTruthy();
  expect(screen.getByText("Aktif → Nonaktif")).toBeTruthy();
  const save = screen.getByRole("button", { name: "Simpan perubahan toko" }) as HTMLButtonElement;
  expect(save.disabled).toBe(true);
  fireEvent.click(screen.getByLabelText("Saya sudah memeriksa peringatan"));
  expect(save.disabled).toBe(false);
  fireEvent.click(save);
  await waitFor(() => expect(toast.success).toHaveBeenCalledWith("1 toko diperbarui"));
  expect(api.importBranchCodeFile).toHaveBeenLastCalledWith({ file, step: "commit", fingerprint: "fp1", ack_warnings: true });
  expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "placements"] });
});
