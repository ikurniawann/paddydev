import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NotificationBell } from "./NotificationBell";

const push = vi.fn();
const mark = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("./use-hris-notifications", () => ({
  useHrisNotifications: () => ({
    data: [
      {
        id: "n1", title: "Pengajuan cuti: Budi", message: "Budi mengajukan Cuti Menikah", type: "approval",
        link: "/dashboard/me/persetujuan-tim", is_read: false, created_at: new Date().toISOString(),
      },
    ],
    refetch: vi.fn(),
  }),
  useMarkNotificationsRead: () => ({ mutate: mark, isPending: false, variables: null }),
}));

afterEach(() => {
  cleanup();
  push.mockReset();
  mark.mockReset();
});

// Popover lonceng di HP terpotong di tepi kiri (QA EPIC-088): dropdown kini
// memakai Popover bersama yang menjaga isi di dalam layar. Tes ini menjaga
// perilakunya tetap sama.
describe("NotificationBell", () => {
  it("membuka daftar, menandai dibaca, dan membuka tautan", () => {
    render(<NotificationBell />);
    fireEvent.click(screen.getByRole("button", { name: "Notifikasi, 1 belum dibaca" }));
    expect(screen.getByText("1 belum dibaca")).toBeTruthy();
    fireEvent.click(screen.getByText("Pengajuan cuti: Budi"));
    expect(mark).toHaveBeenCalledWith("n1");
    expect(push).toHaveBeenCalledWith("/dashboard/me/persetujuan-tim");
  });

  it("Tandai semua menandai semua notifikasi", () => {
    render(<NotificationBell />);
    fireEvent.click(screen.getByRole("button", { name: "Notifikasi, 1 belum dibaca" }));
    fireEvent.click(screen.getByRole("button", { name: /Tandai semua/ }));
    expect(mark).toHaveBeenCalledWith(null);
  });
});
