import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GeofenceNoteDialog } from "./geofence-note-dialog";

const info = {
  message: "Anda di luar area PVJ (±334 m, batas 100 m). Isi keterangan minimal 5 karakter untuk tetap absen.",
  distanceM: 334,
  radiusM: 100,
  branchName: "PVJ",
};

afterEach(cleanup);

describe("GeofenceNoteDialog", () => {
  it("wajib keterangan minimal 5 karakter sebelum tetap absen", () => {
    const onSubmit = vi.fn();
    render(<GeofenceNoteDialog info={info} pending={false} onSubmit={onSubmit} onCancel={() => {}} />);
    expect(screen.getByText(/±334 m dari PVJ/)).toBeTruthy();
    const submit = screen.getByRole("button", { name: "Tetap absen" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: " abc " } });
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "  Ambil stok di gudang " } });
    expect(submit.disabled).toBe(false);
    fireEvent.click(submit);
    expect(onSubmit).toHaveBeenCalledWith("Ambil stok di gudang");
  });

  it("menghitung karakter per code point seperti server (Go rune)", () => {
    // 3 emoji = 6 UTF-16 unit tetapi hanya 3 karakter: belum cukup.
    // 5 emoji = 5 karakter: cukup.
    render(<GeofenceNoteDialog info={info} pending={false} onSubmit={() => {}} onCancel={() => {}} />);
    const submit = screen.getByRole("button", { name: "Tetap absen" }) as HTMLButtonElement;
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "😀😀😀" } });
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "😀😀😀😀😀" } });
    expect(submit.disabled).toBe(false);
  });

  it("memakai radius dari server, bukan angka tetap", () => {
    render(
      <GeofenceNoteDialog info={{ ...info, radiusM: 250 }} pending={false} onSubmit={() => {}} onCancel={() => {}} />
    );
    expect(screen.getByText(/batas 250 m/)).toBeTruthy();
    expect(screen.queryByText(/batas 100 m/)).toBeNull();
  });

  it("tidak tampil tanpa info", () => {
    render(<GeofenceNoteDialog info={null} pending={false} onSubmit={() => {}} onCancel={() => {}} />);
    expect(screen.queryByRole("button", { name: "Tetap absen" })).toBeNull();
  });
});
