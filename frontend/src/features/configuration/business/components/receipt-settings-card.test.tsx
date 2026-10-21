import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({ useReceiptSettings: vi.fn(), useSaveReceiptSettings: vi.fn() }));
vi.mock("../queries", () => ({ useReceiptSettings: mocks.useReceiptSettings }));
vi.mock("../mutations", () => ({ useSaveReceiptSettings: mocks.useSaveReceiptSettings }));

import { ReceiptSettingsCard } from "./receipt-settings-card";

afterEach(cleanup);

describe("ReceiptSettingsCard scope select", () => {
  it("memakai label outlet agar Main Storage tiap branch bisa dibedakan", () => {
    mocks.useSaveReceiptSettings.mockReturnValue({ isPending: false, mutateAsync: vi.fn() });
    mocks.useReceiptSettings.mockReturnValue({
      isLoading: false,
      data: {
        data: [],
        stalls: [
          { id: "a", name: "Main Storage", branch_id: "ba", branch_name: "Alpha", label: "Main Storage — Alpha" },
          { id: "b", name: "Main Storage", branch_id: "bb", branch_name: "Beta", label: "Main Storage — Beta" },
          { id: "c", name: "Stall 1", branch_id: "bb" },
        ],
      },
    });
    render(<ReceiptSettingsCard />);
    expect(screen.getByRole("option", { name: "Main Storage — Alpha" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Main Storage — Beta" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Stall 1" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Main Storage" })).not.toBeInTheDocument();
  });
});
