import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Button } from "@/components/ui/button";
import { leaveAttachmentHref } from "@/lib/hris/attachments";
import { AttachmentPreviewButtons } from "./attachment-preview";
import { RequestDetailDialog } from "./request-detail-dialog";

afterEach(cleanup);

const longReason = "Demam tinggi sejak malam.\nSudah ke dokter, surat dokter terlampir dan istirahat 2 hari sesuai anjuran dokter.";

describe("RequestDetailDialog (EPIC-072)", () => {
  it("menampilkan alasan utuh, lampiran dengan pratinjau, dan aksi di kaki dialog", () => {
    const onApprove = vi.fn();
    render(
      <RequestDetailDialog
        open
        onOpenChange={() => {}}
        title="Detail cuti"
        employee={{ name: "Crew A", nip: "P-001", subtitle: "Shopkeeper Staff" }}
        fields={[{ label: "Jenis", value: "Sakit" }]}
        reason={{ text: longReason }}
        attachments={{ paths: ["leave-attachments/e1/surat dokter.png"], hrefFor: leaveAttachmentHref, label: "Lampiran" }}
        actions={<Button onClick={onApprove}>Setujui</Button>}
      />
    );
    const reason = screen.getByText(/Demam tinggi sejak malam\./);
    expect(reason.textContent).toBe(longReason);
    expect(reason.className).toContain("whitespace-pre-line");
    expect(reason.className).not.toContain("truncate");
    expect(screen.getByText("Crew A")).toBeTruthy();
    expect(screen.getByText("P-001 · Shopkeeper Staff")).toBeTruthy();
    expect(screen.getByText("Sakit")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Lampiran 1" }));
    expect(screen.getByAltText("Lampiran 1").getAttribute("src")).toBe(
      "/api/hris/leaves/attachment/leave-attachments/e1/surat%20dokter.png");
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

    const footer = document.querySelector('[data-slot="dialog-footer"]') as HTMLElement;
    fireEvent.click(within(footer).getByRole("button", { name: "Setujui" }));
    expect(onApprove).toHaveBeenCalled();
  });

  it("tanpa alasan menampilkan tanda kosong, tanpa lampiran tidak merender bagian dokumentasi, tanpa aksi ada Tutup", () => {
    const onOpenChange = vi.fn();
    render(
      <RequestDetailDialog open onOpenChange={onOpenChange} title="Detail lembur" employee={{ name: "Crew B" }}
        fields={[]} reason={{ text: "  " }} attachments={{ paths: [], hrefFor: leaveAttachmentHref }} />
    );
    expect(screen.getByText("Alasan")).toBeTruthy();
    expect(screen.getByText("—")).toBeTruthy();
    expect(screen.queryByText("Dokumentasi")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Tutup" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("AttachmentPreviewButtons", () => {
  it("memakai hrefFor dan label sendiri; PDF dibuka dalam bingkai", () => {
    render(<AttachmentPreviewButtons paths={["a/e1/x.pdf"]} hrefFor={(p) => `/files/${p}`} label="Bukti" />);
    fireEvent.click(screen.getByRole("button", { name: "Bukti 1" }));
    expect(screen.getByTitle("Bukti 1").getAttribute("src")).toBe("/files/a/e1/x.pdf");
  });
});

describe("AttachmentPreviewButtons — nama aksesibel per baris (EPIC-072 sisa)", () => {
  it("context membedakan tombol antarbaris; teks tampilan tetap singkat", () => {
    render(<AttachmentPreviewButtons paths={["a/e1/x.png"]} hrefFor={(p) => `/f/${p}`} label="Lampiran" context="cuti Budi" />);
    const btn = screen.getByRole("button", { name: "Lampiran 1 cuti Budi" });
    expect(btn.textContent).toBe("Lampiran 1");
  });
});

