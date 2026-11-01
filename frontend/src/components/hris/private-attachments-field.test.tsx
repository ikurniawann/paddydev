import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PrivateAttachmentsField } from "./private-attachments-field";

afterEach(cleanup);

function Harness({ upload, initial = [] }: { upload: (f: File) => Promise<string>; initial?: string[] }) {
  const [value, setValue] = useState<string[]>(initial);
  return (
    <>
      <PrivateAttachmentsField
        value={value}
        onChange={setValue}
        upload={upload}
        label="Jaminan"
        noun="jaminan"
        inputLabel="Pilih file jaminan"
        tooManyMessage="Maksimal 3 file jaminan"
      />
      <output data-testid="value">{value.join(",")}</output>
    </>
  );
}

const file = (name: string) => new File(["%PDF-1.4"], name, { type: "application/pdf" });

describe("PrivateAttachmentsField (EPIC-076 review)", () => {
  it("file yang terunggah tetap masuk walau file berikutnya gagal", async () => {
    const upload = vi.fn().mockResolvedValueOnce("a.pdf").mockRejectedValueOnce(new Error("Ukuran lampiran maksimal 5 MB"));
    render(<Harness upload={upload} />);
    fireEvent.change(screen.getByLabelText("Pilih file jaminan"), { target: { files: [file("1.pdf"), file("2.pdf")] } });
    await waitFor(() => expect(screen.getByTestId("value").textContent).toBe("a.pdf"));
    expect(screen.getByRole("alert").textContent).toBe("Ukuran lampiran maksimal 5 MB");
  });

  it("hasil unggahan ditambahkan ke daftar terbaru, bukan daftar saat unggahan dimulai", async () => {
    let finish: (p: string) => void = () => {};
    const upload = vi.fn(() => new Promise<string>((resolve) => (finish = resolve)));
    function Outer() {
      const [value, setValue] = useState<string[]>(["lama.pdf"]);
      return (
        <>
          <button onClick={() => setValue([])}>kosongkan</button>
          <PrivateAttachmentsField value={value} onChange={setValue} upload={upload} label="J" noun="jaminan"
            inputLabel="Pilih file jaminan" tooManyMessage="x" />
          <output data-testid="value">{value.join(",")}</output>
        </>
      );
    }
    render(<Outer />);
    fireEvent.change(screen.getByLabelText("Pilih file jaminan"), { target: { files: [file("1.pdf")] } });
    expect(screen.getByRole("button", { name: "Hapus jaminan 1" }).hasAttribute("disabled")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "kosongkan" }));
    await act(async () => finish("baru.pdf"));
    expect(screen.getByTestId("value").textContent).toBe("baru.pdf");
  });
});
