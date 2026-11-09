import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { UserAvatar } from "./user-avatar";

afterEach(cleanup);

describe("UserAvatar (EPIC-087)", () => {
  it("tanpa foto menampilkan inisial huruf pertama", () => {
    render(<UserAvatar name="paddy HR" />);
    expect(screen.getByText("P")).toBeTruthy();
  });

  it("nama kosong jatuh ke A", () => {
    render(<UserAvatar name="" />);
    expect(screen.getByText("A")).toBeTruthy();
  });

  it("dengan foto menampilkan img, kembali ke inisial bila gagal dimuat", () => {
    const { container } = render(<UserAvatar name="Paddy HR" src="/api/files/profile-photos/u/a.jpg" />);
    const img = container.querySelector("img");
    expect(img?.getAttribute("src")).toContain("/api/files/profile-photos/u/a.jpg");
    fireEvent.error(img!);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("P")).toBeTruthy();
  });
});
