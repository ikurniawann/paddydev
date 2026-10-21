import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({ confirmAndSwitchStall: vi.fn() }));

vi.mock("next/navigation", () => ({
  usePathname: () => "/dashboard",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
}));
vi.mock("@/components/pos/confirm-stall-switch-dialog", () => ({
  useConfirmAndSwitchStall: () => ({
    confirmAndSwitchStall: mocks.confirmAndSwitchStall,
    switching: false,
    dialogOpen: false,
    dialog: null,
  }),
  useCanUseCentralCashier: () => false,
}));

import { StallSwitcher } from "./app-sidebar";

function stubStallOptions(stalls: unknown[], allAccess = true) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      json: async () => ({ success: true, data: { all_access: allAccess, can_switch: true, stalls } }),
    }))
  );
}

async function openSwitcher() {
  render(
    <StallSwitcher activeStallId={null} canUseCentralCashier={false}>
      <span>Lokasi</span>
    </StallSwitcher>
  );
  fireEvent.click(screen.getByRole("button", { name: "Ganti outlet" }));
}

beforeEach(() => {
  mocks.confirmAndSwitchStall.mockReset();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("StallSwitcher", () => {
  it("memakai label dari server sehingga Main Storage tiap branch bisa dibedakan", async () => {
    stubStallOptions([
      { id: "m-a", name: "Main Storage", code: "MAIN", branch_name: "Alpha", label: "Main Storage — Alpha" },
      { id: "m-b", name: "Main Storage", code: "MAIN", branch_name: "Beta", label: "Main Storage — Beta" },
    ]);
    await openSwitcher();

    expect(await screen.findByText("Main Storage — Alpha")).toBeInTheDocument();
    expect(screen.getByText("Main Storage — Beta")).toBeInTheDocument();
    expect(screen.queryByText("Main Storage")).not.toBeInTheDocument();
    expect(screen.getByText("Semua Outlet")).toBeInTheDocument();
  });

  it("jatuh ke nama bila server tidak mengirim label", async () => {
    stubStallOptions([{ id: "s1", name: "Stall 1", code: "STALL-01" }]);
    await openSwitcher();

    expect(await screen.findByText("Stall 1")).toBeInTheDocument();
  });

  it("memilih baris tetap mengirim id outlet-nya", async () => {
    stubStallOptions([
      { id: "m-b", name: "Main Storage", code: "MAIN", label: "Main Storage — Beta" },
    ]);
    await openSwitcher();
    fireEvent.click(await screen.findByText("Main Storage — Beta"));

    await waitFor(() => expect(mocks.confirmAndSwitchStall).toHaveBeenCalledWith("m-b"));
  });
});
