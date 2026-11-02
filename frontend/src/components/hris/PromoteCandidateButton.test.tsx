// EPIC-081: promote kandidat → karyawan memakai NIP yang diketik HR (wajib, tidak digenerate).
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PromoteCandidateButton } from "./PromoteCandidateButton";

const mocks = vi.hoisted(() => ({ apiPost: vi.fn(), toastError: vi.fn(), toastSuccess: vi.fn() }));
vi.mock("@/lib/api-client", () => ({ apiPost: mocks.apiPost }));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: mocks.toastSuccess } }));

function renderButton(onSuccess = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <PromoteCandidateButton
        candidate={{ id: "cand-1", full_name: "Sari Dewi", status: "hired", email: "sari@x.id", phone: "0812" }}
        onSuccess={onSuccess}
      />
    </QueryClientProvider>
  );
  fireEvent.click(screen.getByRole("button", { name: /Promote to Employee/ }));
  return onSuccess;
}

const nipInput = () => screen.getByLabelText(/NIP/) as HTMLInputElement;
const submit = () => fireEvent.click(screen.getByRole("button", { name: /Promote Sekarang/ }));

beforeEach(() => {
  mocks.apiPost.mockReset().mockResolvedValue({ nip: "202511001", contract_number: null });
  mocks.toastError.mockClear();
  mocks.toastSuccess.mockClear();
});

describe("PromoteCandidateButton — NIP manual wajib", () => {
  it("menampilkan kolom NIP wajib dan tidak lagi menjanjikan NIP otomatis", () => {
    renderButton();
    expect(nipInput().placeholder).toBe("mis. 202511001");
    expect(nipInput()).toBeRequired();
    expect(screen.queryByText(/digenerate|otomatis \(format/i)).not.toBeInTheDocument();
  });

  it("NIP kosong: error di kolom dan tidak dikirim", () => {
    renderButton();
    fireEvent.change(nipInput(), { target: { value: "   " } });
    submit();
    expect(screen.getByText("NIP wajib diisi")).toBeInTheDocument();
    expect(nipInput()).toHaveAttribute("aria-invalid", "true");
    expect(mocks.apiPost).not.toHaveBeenCalled();
  });

  it("NIP dikirim (dipangkas) bersama data promote", async () => {
    const onSuccess = renderButton();
    fireEvent.change(nipInput(), { target: { value: " 202511001 " } });
    submit();
    await waitFor(() => expect(mocks.apiPost).toHaveBeenCalled());
    expect(mocks.apiPost.mock.calls[0][0]).toBe("/api/hris/promote");
    expect(mocks.apiPost.mock.calls[0][1]).toMatchObject({ candidate_id: "cand-1", nip: "202511001" });
    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
  });

  it("bentrok NIP dari server tampil di kolom NIP", async () => {
    mocks.apiPost.mockRejectedValue(new Error("NIP sudah dipakai karyawan Budi (NIP 1, aktif)"));
    renderButton();
    fireEvent.change(nipInput(), { target: { value: "1" } });
    submit();
    expect(await screen.findByText("NIP sudah dipakai karyawan Budi (NIP 1, aktif)")).toBeInTheDocument();
  });
});
