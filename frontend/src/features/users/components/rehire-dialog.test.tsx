import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { RehireDialog, type RehireEmployee } from "./rehire-dialog";

const api = vi.hoisted(() => ({
  rehireEmployee: vi.fn(async (..._a: unknown[]) => ({
    data: { id: "e1" } as Record<string, unknown>,
    message: "Karyawan diaktifkan kembali",
  })),
}));
vi.mock("../api", () => api);
vi.mock("../queries", () => ({
  useUserFormLookups: () => ({
    data: {
      employmentStatuses: [
        { code: "permanent", name: "Tetap" },
        { code: "contract", name: "Kontrak" },
        { code: "resigned", name: "Resign" },
        { code: "terminated", name: "Diberhentikan" },
      ],
    },
  }),
}));
vi.mock("@/features/hris/placements/queries", () => ({
  useStoreBranches: () => ({
    isLoading: false,
    data: [
      { id: "b1b1b1b1-0000-0000-0000-000000000001", code: "JKT", name: "Jakarta" },
      { id: "b2b2b2b2-0000-0000-0000-000000000002", code: "BDG", name: "Bandung" },
    ],
  }),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const base: RehireEmployee = {
  id: "e1",
  full_name: "Rina",
  user_id: null,
  email: "rina@x.id",
  join_date: "2025-01-10",
  end_date: "2026-06-30",
};

function open(employee: Partial<RehireEmployee> = {}, onDone = vi.fn()) {
  render(
    <RehireDialog open employee={{ ...base, ...employee }} today="2026-10-21" onOpenChange={() => {}} onDone={onDone} />
  );
  return onDone;
}

function fillRequired() {
  fireEvent.change(screen.getByLabelText("Status kepegawaian *"), { target: { value: "contract" } });
  fireEvent.change(screen.getByLabelText("Cabang penempatan *"), { target: { value: "b1b1b1b1-0000-0000-0000-000000000001" } });
}

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

it("karyawan tanpa login: ada input email, tidak ada checkbox akun", () => {
  open({ user_id: null });
  expect((screen.getByLabelText("Email") as HTMLInputElement).value).toBe("rina@x.id");
  expect(screen.queryByLabelText("Aktifkan kembali akun login")).toBeNull();
});

it("karyawan dengan login: ada checkbox akun (tercentang), tidak ada input email", () => {
  open({ user_id: "u1" });
  expect((screen.getByLabelText("Aktifkan kembali akun login") as HTMLInputElement).checked).toBe(true);
  expect(screen.queryByLabelText("Email")).toBeNull();
});

it.each(["lama@tanpa-email.invalid", "arsip-x@resign.invalid"])("email placeholder %s dikosongkan", (email) => {
  open({ email });
  expect((screen.getByLabelText("Email") as HTMLInputElement).value).toBe("");
});

it("status resigned/terminated tidak ditawarkan", () => {
  open();
  const values = Array.from((screen.getByLabelText("Status kepegawaian *") as HTMLSelectElement).options).map((o) => o.value);
  expect(values).toContain("permanent");
  expect(values).toContain("contract");
  expect(values).not.toContain("resigned");
  expect(values).not.toContain("terminated");
});

it("tanggal efektif: min = hari setelah tanggal keluar, max = hari ini", () => {
  open();
  const date = screen.getByLabelText("Tanggal efektif *") as HTMLInputElement;
  expect(date.min).toBe("2026-07-01");
  expect(date.max).toBe("2026-10-21");
});

it("tanpa tanggal keluar: min = hari setelah tanggal masuk", () => {
  open({ end_date: null });
  expect((screen.getByLabelText("Tanggal efektif *") as HTMLInputElement).min).toBe("2025-01-11");
});

it("tombol nonaktif sampai status & cabang dipilih, dan menolak tanggal di luar rentang", () => {
  open();
  const submit = screen.getByRole("button", { name: "Aktifkan kembali" }) as HTMLButtonElement;
  expect(submit.disabled).toBe(true);
  fillRequired();
  expect(submit.disabled).toBe(false);
  fireEvent.change(screen.getByLabelText("Tanggal efektif *"), { target: { value: "2026-06-30" } });
  expect(screen.getByText(/Harus setelah tanggal keluar/)).toBeTruthy();
  expect(submit.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Tanggal efektif *"), { target: { value: "2026-11-05" } });
  expect(screen.getByText(/Tidak boleh setelah hari ini/)).toBeTruthy();
});

it("tanpa login: mengirim body lengkap dengan email", async () => {
  const onDone = open();
  fillRequired();
  fireEvent.change(screen.getByLabelText("Tanggal efektif *"), { target: { value: "2026-10-01" } });
  fireEvent.change(screen.getByLabelText("Email"), { target: { value: " baru@x.id " } });
  fireEvent.change(screen.getByLabelText("Alasan"), { target: { value: "Dipanggil lagi" } });
  fireEvent.click(screen.getByRole("button", { name: "Aktifkan kembali" }));
  await waitFor(() =>
    expect(api.rehireEmployee).toHaveBeenCalledWith("e1", {
      effective_date: "2026-10-01",
      employment_status: "contract",
      branch_id: "b1b1b1b1-0000-0000-0000-000000000001",
      email: "baru@x.id",
      reason: "Dipanggil lagi",
    })
  );
  expect(toast.success).toHaveBeenCalledWith("Karyawan diaktifkan kembali");
  expect(onDone).toHaveBeenCalled();
});

it("dengan login: mengirim reactivate_login dan TIDAK mengirim email", async () => {
  open({ user_id: "u1" });
  fillRequired();
  fireEvent.click(screen.getByLabelText("Aktifkan kembali akun login"));
  fireEvent.click(screen.getByRole("button", { name: "Aktifkan kembali" }));
  await waitFor(() => expect(api.rehireEmployee).toHaveBeenCalled());
  const body = api.rehireEmployee.mock.calls[0][1] as Record<string, unknown>;
  expect(body.reactivate_login).toBe(false);
  expect("email" in body).toBe(false);
  expect("reason" in body).toBe(false);
});

it("email kosong tidak dikirim", async () => {
  open({ email: "x@tanpa-email.invalid" });
  fillRequired();
  fireEvent.click(screen.getByRole("button", { name: "Aktifkan kembali" }));
  await waitFor(() => expect(api.rehireEmployee).toHaveBeenCalled());
  expect("email" in (api.rehireEmployee.mock.calls[0][1] as object)).toBe(false);
});

it("catatan jadwal shift ikut ditampilkan di toast", async () => {
  api.rehireEmployee.mockResolvedValueOnce({
    data: { id: "e1", schedule_note: "Atur jadwal shift karyawan ini di Jadwal Kerja" },
    message: "Karyawan diaktifkan kembali",
  });
  open();
  fillRequired();
  fireEvent.click(screen.getByRole("button", { name: "Aktifkan kembali" }));
  await waitFor(() =>
    expect(toast.success).toHaveBeenCalledWith("Karyawan diaktifkan kembali", {
      description: "Atur jadwal shift karyawan ini di Jadwal Kerja",
    })
  );
});

it("galat server muncul di toast dan dialog tidak selesai", async () => {
  api.rehireEmployee.mockRejectedValueOnce(new Error("Email sudah digunakan"));
  const onDone = open();
  fillRequired();
  fireEvent.click(screen.getByRole("button", { name: "Aktifkan kembali" }));
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Email sudah digunakan"));
  expect(onDone).not.toHaveBeenCalled();
});
