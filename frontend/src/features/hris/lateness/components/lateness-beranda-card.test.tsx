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
    expect(screen.getByText("Teguran lisan dan tertulis")).toBeTruthy();
    expect(screen.getByText("2 kali terlambat lagi → Surat Peringatan 1")).toBeTruthy();
  });

  it("belum dimuat: tidak dirender", () => {
    const { container } = render(<LatenessBerandaCard />);
    expect(container.innerHTML).toBe("");
  });
});
