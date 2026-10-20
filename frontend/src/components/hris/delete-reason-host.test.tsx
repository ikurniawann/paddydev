import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { DeleteCancelledError, askDeleteReason } from "@/lib/hris/delete-reason";
import { DeleteReasonHost } from "./delete-reason-host";

afterEach(cleanup);

it("meminta alasan ≥ 5 karakter lalu mengembalikannya", async () => {
  render(<DeleteReasonHost />);
  let pending!: Promise<string>;
  act(() => {
    pending = askDeleteReason({ title: "Hapus shift" });
  });
  expect(screen.getByText("Hapus shift")).toBeTruthy();
  const confirm = screen.getByRole("button", { name: "Hapus" }) as HTMLButtonElement;
  expect(confirm.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText(/Alasan/), { target: { value: "Shift dobel" } });
  expect(confirm.disabled).toBe(false);
  act(() => confirm.click());
  await expect(pending).resolves.toBe("Shift dobel");
});

it("Batal menolak dengan DeleteCancelledError", async () => {
  render(<DeleteReasonHost />);
  let pending!: Promise<string>;
  act(() => {
    pending = askDeleteReason({ title: "Hapus libur" });
  });
  act(() => screen.getByRole("button", { name: "Batal" }).click());
  await expect(pending).rejects.toBeInstanceOf(DeleteCancelledError);
});
