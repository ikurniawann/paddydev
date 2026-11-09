import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MyLateness } from "../types";
import { LatenessBerandaCard } from "./lateness-beranda-card";
import { lateDay, myLateness } from "./test-fixtures";

let data: MyLateness | undefined;
vi.mock("../queries", () => ({ useMyLateness: () => ({ data }) }));

afterEach(() => {
  cleanup();
  data = undefined;
});

describe("LatenessBerandaCard (EPIC-077 R9)", () => {
  it("ringkasan periode ini dan tautan ke kartu lengkap", () => {
    data = myLateness("teguran", [lateDay("2026-10-02")], { code: "sp1", label: "Surat Peringatan 1", at: 3, remaining: 2 });
    render(<LatenessBerandaCard />);
    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/dashboard/me/absensi#keterlambatan");
    expect(screen.getByText("Keterlambatan periode ini")).toBeTruthy();
    // EPIC-094: tanpa label Teguran/SP, jumlah dan pengingat tetap ada.
    expect(screen.getByText("1")).toBeTruthy();
    expect(screen.getByText("Hati-hati, keterlambatan yang berlanjut dapat berujung teguran dari HR.")).toBeTruthy();
    expect(screen.queryByText(/Surat Peringatan|Teguran lisan|kali terlambat lagi/)).toBeNull();
  });

  it("belum dimuat: tidak dirender", () => {
    const { container } = render(<LatenessBerandaCard />);
    expect(container.innerHTML).toBe("");
  });
});
