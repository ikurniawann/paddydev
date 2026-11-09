"use client";

import { useMemo, useState } from "react";
import { Loader2, Paperclip, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox, type ComboboxOption } from "@/components/ui/combobox";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { searchPlacements } from "@/features/hris/placements/api";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { todayWib } from "@/lib/dates";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { uploadLetterAttachment } from "../api";
import { useIssueLetter } from "../mutations";
import { useLetterLateness } from "../queries";
import { LETTER_TYPES, type LetterType } from "../types";

interface IssueLetterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Dari profil karyawan: karyawan terkunci. */
  employeeId?: string;
  employeeName?: string;
}

/** HR menerbitkan Surat Teguran/SP (EPIC-094 R9–R11). */
export function IssueLetterDialog(props: IssueLetterDialogProps) {
  const { open, onOpenChange } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>Terbitkan surat</DialogPanelTitle>
        </DialogPanelHeader>
        {open && <IssueLetterBody {...props} />}
      </DialogPanel>
    </Dialog>
  );
}

function IssueLetterBody({ onOpenChange, employeeId: lockedId, employeeName }: IssueLetterDialogProps) {
  const today = todayWib();
  const [employeeId, setEmployeeId] = useState(lockedId ?? "");
  const [picked, setPicked] = useState<ComboboxOption | null>(null);
  const [searchText, setSearchText] = useState("");
  const q = useDebouncedValue(searchText.trim(), 300);
  const [type, setType] = useState<LetterType>("teguran");
  const [issued, setIssued] = useState(today);
  const [notes, setNotes] = useState("");
  const [attachment, setAttachment] = useState<{ path: string; name: string } | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const issue = useIssueLetter();
  const lateness = useLetterLateness(employeeId || undefined);

  const employees = useQuery({
    queryKey: ["hris", "disciplinary-letter-employees", q],
    queryFn: () => searchPlacements({ q: q || undefined, page: 1, page_size: 25 }),
    enabled: !lockedId,
    placeholderData: keepPreviousData,
  });
  const options = useMemo(() => {
    const list: ComboboxOption[] = (employees.data?.rows ?? []).map((r) => ({
      value: r.employee_id,
      label: r.nip ? `${r.full_name} · ${r.nip}` : r.full_name,
      description: r.position_title || undefined,
    }));
    if (picked && !list.some((o) => o.value === picked.value)) list.unshift(picked);
    return list;
  }, [employees.data, picked]);

  const future = issued > today;
  const label = LETTER_TYPES.find((t) => t.value === type)?.label ?? "";

  function pickEmployee(id: string) {
    setEmployeeId(id);
    setPicked(options.find((o) => o.value === id) ?? null);
    // Lampiran tersimpan atas nama karyawan sebelumnya; server menolaknya untuk karyawan lain.
    setAttachment(null);
    setError("");
  }

  async function pickFile(file: File | undefined) {
    if (!file || !employeeId) return;
    setUploading(true);
    setError("");
    try {
      setAttachment({ path: await uploadLetterAttachment(file, employeeId), name: file.name });
    } catch (e) {
      setError(e instanceof Error && e.message ? e.message : "Gagal mengunggah lampiran");
    } finally {
      setUploading(false);
    }
  }

  function handleSave() {
    if (!employeeId) {
      setError("Pilih karyawan");
      return;
    }
    setError("");
    issue.mutate(
      {
        employee_id: employeeId,
        letter_type: type,
        issued_date: issued,
        notes: notes.trim() || null,
        attachment_path: attachment?.path ?? null,
      },
      {
        onSuccess: () => {
          toast.success(`${label} diterbitkan`);
          onOpenChange(false);
        },
        onError: (e: Error) => setError(e.message || "Gagal menerbitkan surat"),
      }
    );
  }

  return (
    <>
      <DialogPanelBody className="min-w-0 space-y-4">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Karyawan</label>
          {lockedId ? (
            <p className="rounded-lg border border-gray-200/80 bg-gray-50 px-3 py-2 text-sm font-medium text-gray-900">
              {employeeName}
            </p>
          ) : (
            <Combobox
              options={options}
              value={employeeId}
              onChange={pickEmployee}
              onSearchChange={setSearchText}
              placeholder="Pilih karyawan"
              searchPlaceholder="Cari nama atau NIP..."
              emptyMessage={employees.isFetching ? "Mencari..." : "Karyawan tidak ditemukan."}
            />
          )}
          {lateness.data ? (
            <p className="mt-1 text-xs text-gray-600">
              {lateness.data.count > 0
                ? `Terlambat ${lateness.data.count} kali di periode ini (ketentuan: ${lateness.data.label})`
                : "Tidak ada keterlambatan di periode ini"}
            </p>
          ) : null}
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label htmlFor="letter-type" className="mb-1 block text-xs font-medium text-gray-600">
              Jenis surat
            </label>
            <select
              id="letter-type"
              aria-label="Jenis surat"
              value={type}
              onChange={(e) => setType(e.target.value as LetterType)}
              className="h-10 w-full rounded-lg border border-gray-200 bg-white px-3 text-sm"
            >
              {LETTER_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="letter-date" className="mb-1 block text-xs font-medium text-gray-600">
              Tanggal terbit
            </label>
            <Input
              id="letter-date"
              aria-label="Tanggal terbit"
              type="date"
              max={today}
              value={issued}
              onChange={(e) => setIssued(e.target.value)}
            />
          </div>
        </div>
        {future ? <p className="text-xs text-red-600">Tanggal terbit tidak boleh di masa depan</p> : null}

        <div>
          <label htmlFor="letter-notes" className="mb-1 block text-xs font-medium text-gray-600">
            Catatan
          </label>
          <Textarea
            id="letter-notes"
            aria-label="Catatan"
            rows={3}
            maxLength={2000}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Mis. terlambat 3 kali di periode Oktober"
          />
        </div>

        <div>
          <label htmlFor="letter-file" className="mb-1 block text-xs font-medium text-gray-600">
            Lampiran (opsional)
          </label>
          {attachment ? (
            <div className="flex min-w-0 items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm">
              <Paperclip className="h-4 w-4 shrink-0 text-gray-400" />
              <span className="min-w-0 flex-1 truncate">{attachment.name}</span>
              <button type="button" aria-label="Hapus lampiran" onClick={() => setAttachment(null)}>
                <X className="h-4 w-4 text-gray-500" />
              </button>
            </div>
          ) : (
            <Input
              id="letter-file"
              aria-label="Lampiran (opsional)"
              type="file"
              accept="application/pdf,image/jpeg,image/png,image/webp"
              disabled={!employeeId || uploading}
              onChange={(e) => pickFile(e.target.files?.[0])}
            />
          )}
          <p className="mt-1 text-xs text-gray-500">PDF atau foto, maksimal 5 MB.</p>
        </div>

        <p className="text-xs text-gray-500">
          Lonceng masuk ke karyawan, atasannya (SPV dan Manager), dan HR saat surat diterbitkan.
        </p>

        {error && (
          <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </p>
        )}
      </DialogPanelBody>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={issue.isPending}>
          Batal
        </Button>
        <Button onClick={handleSave} disabled={issue.isPending || uploading || future || !issued}>
          {issue.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Terbitkan"}
        </Button>
      </DialogFooter>
    </>
  );
}
