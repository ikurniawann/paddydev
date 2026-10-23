import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { OvertimeAttachmentButtons } from "./overtime-attachment-preview";

afterEach(cleanup);

const PATHS = [
  "overtime-attachments/e1/1-abcdefabcdefabcd.jpg",
  "overtime-attachments/e1/2-abcdefabcdefabcd.pdf",
];

describe("OvertimeAttachmentButtons", () => {
  it("tidak memakai tautan tab baru", () => {
    const { container } = render(<OvertimeAttachmentButtons paths={PATHS} />);
    expect(container.querySelector('a[target="_blank"]')).toBeNull();
    expect(screen.getAllByRole("button", { name: /Dokumentasi \d/ })).toHaveLength(2);
  });

  it("gambar dibuka di modal sebagai <img>, PDF sebagai iframe, bisa berpindah", () => {
    render(<OvertimeAttachmentButtons paths={PATHS} />);
    fireEvent.click(screen.getByRole("button", { name: "Dokumentasi 1" }));
    const dialog = screen.getByRole("dialog");
    const img = within(dialog).getByRole("img", { name: "Dokumentasi 1" });
    expect(img.getAttribute("src")).toBe("/api/hris/overtime/attachment/overtime-attachments/e1/1-abcdefabcdefabcd.jpg");
    expect(within(dialog).getByText("1 / 2")).toBeTruthy();

    fireEvent.click(within(dialog).getByRole("button", { name: "Berikutnya" }));
    const frame = within(dialog).getByTitle("Dokumentasi 2");
    expect(frame.tagName).toBe("IFRAME");
    expect(frame.getAttribute("src")).toBe("/api/hris/overtime/attachment/overtime-attachments/e1/2-abcdefabcdefabcd.pdf");
    expect((within(dialog).getByRole("button", { name: "Berikutnya" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("tanpa dokumentasi tidak menampilkan apa pun", () => {
    const { container } = render(<OvertimeAttachmentButtons paths={[]} />);
    expect(container.textContent).toBe("");
  });
});
