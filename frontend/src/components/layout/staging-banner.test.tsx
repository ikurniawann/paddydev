import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StagingBanner } from "./staging-banner";

describe("StagingBanner", () => {
  it("tidak merender apa pun tanpa URL production", () => {
    const { container } = render(<StagingBanner />);
    expect(container.innerHTML).toBe("");
  });

  it("tidak merender apa pun bila URL bukan https", () => {
    const { container } = render(<StagingBanner productionUrl="javascript:alert(1)" />);
    expect(container.innerHTML).toBe("");
  });

  it("menandai STAGING dan menautkan ke production", () => {
    render(<StagingBanner productionUrl="https://paddyadorable.biz.id" />);
    expect(screen.getByRole("status").textContent).toContain("STAGING");
    const link = screen.getByRole("link", { name: "paddyadorable.biz.id" });
    expect(link.getAttribute("href")).toBe("https://paddyadorable.biz.id/login");
  });
});
