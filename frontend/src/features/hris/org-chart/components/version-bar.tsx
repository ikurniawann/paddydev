"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { approveOrgChartVersion, fetchOrgChartValidation, fetchOrgChartVersions } from "../api";
import { orgChartQueryKeys } from "../query-keys";
import type { ChartVersion, ChartVersionStatus } from "../types";

const STATUS: Record<ChartVersionStatus, { label: string; color: string }> = {
  draft: { label: "Draft", color: "bg-yellow-100 text-yellow-800" },
  approved: { label: "Disetujui", color: "bg-green-100 text-green-700" },
  archived: { label: "Arsip", color: "bg-gray-100 text-gray-600" },
};

/** Hari ini dalam tanggal lokal perangkat (YYYY-MM-DD). */
function localToday() {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Versi yang ditampilkan: pilihan user, bila kosong sama dengan bawaan server (EPIC-102 R7). */
export function shownVersion(versions: ChartVersion[], version: string): ChartVersion | undefined {
  if (version) return versions.find((v) => v.label === version);
  return versions.find((v) => v.status === "approved") ?? versions[0];
}

function ApproveDialog({ target, open, onOpenChange }: { target: ChartVersion; open: boolean; onOpenChange: (o: boolean) => void }) {
  const qc = useQueryClient();
  const [validFrom, setValidFrom] = useState(localToday);
  const [reason, setReason] = useState("");
  const validation = useQuery({
    queryKey: orgChartQueryKeys.validation(target.label),
    queryFn: () => fetchOrgChartValidation(target.label),
    enabled: open,
  });
  const approve = useMutation({
    mutationFn: () => approveOrgChartVersion(target.id, { valid_from: validFrom, reason: reason.trim() }),
    onSuccess: () => {
      toast.success(`Versi ${target.label} disetujui`);
      onOpenChange(false);
      return qc.invalidateQueries({ queryKey: orgChartQueryKeys.all });
    },
    onError: (e) => toast.error(e instanceof Error && e.message ? e.message : "Gagal menyetujui versi"),
  });
  const errors = validation.data?.errors.length ?? 0;
  const blocked = !validation.data || errors > 0 || !/^\d{4}-\d{2}-\d{2}$/.test(validFrom) || approve.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Setujui {target.label}</DialogTitle>
          <DialogDescription>
            Versi ini menjadi bagan resmi dan tampil sebagai bawaan. Versi yang disetujui sebelumnya diarsipkan. Data
            karyawan, atasan langsung, dan alur approval tidak berubah.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="org-valid-from">Tanggal berlaku</Label>
            <Input id="org-valid-from" type="date" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="org-approve-reason">Alasan (opsional)</Label>
            <Textarea id="org-approve-reason" rows={2} value={reason} onChange={(e) => setReason(e.target.value)} />
          </div>
          {validation.isError ? (
            <p role="alert" className="rounded-lg bg-danger-soft p-2 text-sm text-danger">
              Validasi bagan tidak bisa dimuat
            </p>
          ) : validation.isLoading ? (
            <p className="text-sm text-gray-500">Memeriksa struktur bagan…</p>
          ) : errors > 0 ? (
            <p role="alert" className="rounded-lg bg-danger-soft p-2 text-sm text-danger">
              Perbaiki {errors} kesalahan struktur dulu
            </p>
          ) : validation.data ? (
            <p className="text-sm text-gray-600">
              Struktur bagan valid
              {validation.data.warnings.length > 0 ? ` (${validation.data.warnings.length} peringatan, tidak menghalangi)` : ""}.
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={approve.isPending}>
            Batal
          </Button>
          <Button onClick={() => approve.mutate()} disabled={blocked}>
            Setujui
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Pemilih versi bagan dan persetujuan versi (EPIC-102). Tombol Setujui hanya
 * untuk versi draft; server memeriksa hak "approve" dan error struktur.
 */
export function VersionBar({ version, onChange }: { version: string; onChange: (label: string) => void }) {
  const versions = useQuery({ queryKey: orgChartQueryKeys.versions(), queryFn: fetchOrgChartVersions });
  const [open, setOpen] = useState(false);
  const list = versions.data ?? [];
  const shown = shownVersion(list, version);

  if (versions.isError) {
    return (
      <p role="alert" className="rounded-lg bg-danger-soft p-3 text-sm text-danger">
        Daftar versi bagan tidak bisa dimuat
      </p>
    );
  }
  if (list.length === 0) return null;

  return (
    <div className="flex flex-col gap-2 rounded-lg border bg-white p-3 sm:flex-row sm:items-center sm:gap-3">
      <label className="flex items-center gap-2 text-sm">
        <span className="shrink-0 text-xs font-medium text-gray-600">Versi</span>
        <select
          aria-label="Versi bagan"
          className="h-9 w-full rounded-md border border-gray-300 bg-white px-2 text-sm sm:w-56"
          value={shown?.label ?? ""}
          onChange={(e) => onChange(e.target.value)}
        >
          {list.map((v) => (
            <option key={v.id} value={v.label}>
              {v.label} · {STATUS[v.status]?.label ?? v.status}
            </option>
          ))}
        </select>
      </label>
      {shown ? (
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-xs text-gray-600">
          <Badge className={`px-1.5 py-0 text-xs ${STATUS[shown.status]?.color ?? ""}`}>{STATUS[shown.status]?.label ?? shown.status}</Badge>
          {shown.status !== "draft" && shown.approved_by_name ? (
            <span className="break-words">
              Disetujui {shown.approved_by_name}
              {shown.valid_from ? ` · berlaku sejak ${shown.valid_from}` : ""}
            </span>
          ) : null}
        </div>
      ) : null}
      {shown?.status === "draft" ? (
        <>
          <Button size="sm" onClick={() => setOpen(true)}>
            Setujui versi
          </Button>
          {open ? <ApproveDialog key={shown.id} target={shown} open={open} onOpenChange={setOpen} /> : null}
        </>
      ) : null}
    </div>
  );
}
