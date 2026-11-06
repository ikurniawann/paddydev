import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MyLettersSection } from "./my-letters-section";
import { letter, renderWithClient } from "./test-setup";

const api = vi.hoisted(() => ({ fetchMyLetters: vi.fn() }));
vi.mock("../api", async (importOriginal) => ({ ...(await importOriginal<typeof import("../api")>()), ...api }));

afterEach(cleanup);

describe("MyLettersSection (EPIC-094 R12)", () => {
  it("tanpa surat: tidak merender apa pun", async () => {
    api.fetchMyLetters.mockResolvedValue([]);
    const { container } = renderWithClient(<MyLettersSection />);
    await new Promise((r) => setTimeout(r, 20));
    expect(container.innerHTML).toBe("");
  });

  it("menampilkan surat dari HR dengan tanggal, catatan, dan lampiran", async () => {
    api.fetchMyLetters.mockResolvedValue([letter({ attachment_path: "disciplinary-letters/e1/1-abcdef0123456789.pdf" })]);
    renderWithClient(<MyLettersSection />);
    expect(await screen.findByText("Surat dari HR")).toBeTruthy();
    expect(screen.getByText("Surat Peringatan 1")).toBeTruthy();
    expect(screen.getByText("05-11-2026")).toBeTruthy();
    expect(screen.getByText("Terlambat 3 kali")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Lihat lampiran" }).getAttribute("href")).toBe(
      "/api/hris/disciplinary-letters/attachment/disciplinary-letters/e1/1-abcdef0123456789.pdf"
    );
    expect(document.getElementById("surat")).toBeTruthy();
  });

  it("tautan lonceng #surat: menggulir ke bagian ini sekali setelah data tampil", async () => {
    api.fetchMyLetters.mockResolvedValue([letter()]);
    const scroll = vi.fn();
    Element.prototype.scrollIntoView = scroll;
    window.location.hash = "#surat";
    renderWithClient(<MyLettersSection />);
    await screen.findByText("Surat dari HR");
    await waitFor(() => expect(scroll).toHaveBeenCalledTimes(1));
    window.location.hash = "";
  });
});
