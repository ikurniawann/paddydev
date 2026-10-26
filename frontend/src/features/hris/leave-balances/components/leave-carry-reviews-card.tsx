"use client";

import { useEffect, useState } from "react";
import { AlertTriangle, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/format";
import { formatDays } from "../balance-math";
import { useResolveCarryReview } from "../mutations";
import { useCarryReviews, useLeaveBalance } from "../queries";
import type { CarryReview } from "../types";
import { LeaveBalanceEditDialog } from "./leave-balance-edit-dialog";

const NOTE_MAX = 500;

const CAUSES: Record<string, string> = {
  leave_approve: "Cuti disetujui",
  leave_cancel: "Cuti dibatalkan",
  leave_reject: "Cuti ditolak",
  hr_update: "Koreksi HR",
  import: "Import massal",
  settings: "Batas sisa bawaan diubah",
  job: "Job 1 Januari",
};

/** Penyebab terbaca dari `cause.type`; jenis yang belum dikenal memakai kalimat umum. */
const causeText = (r: CarryReview) => CAUSES[r.cause?.type ?? ""] ?? "Hitung ulang sisa bawaan";

function ResolveDialog({ review, onClose }: { review: CarryReview; onClose: () => void }) {
  const resolve = useResolveCarryReview();
  const [note, setNote] = useState("");
  const [serverError, setServerError] = useState("");
  const trimmed = note.trim();
  const tooLong = trimmed.length > NOTE_MAX;

  function submit() {
    if (trimmed === "" || tooLong) return;
    resolve.mutate(
      { id: review.id, note: trimmed },
      {
        onSuccess: (res) => {
          toast.success(res.message || "Tinjauan ditandai selesai");
          onClose();
        },
        onError: (err: Error) => setServerError(err.message || "Gagal menandai selesai"),
      }
    );
  }

  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogPanel size="md">
        <DialogPanelHeader>
          <DialogPanelTitle>Tandai selesai</DialogPanelTitle>
          <DialogPanelDescription>
            {review.full_name}
            {review.nip ? ` (${review.nip})` : ""}, {review.year}: sisa bawaan {formatDays(review.carry_over_days)} hari,
            terpakai {formatDays(review.carry_over_used)} hari, kelebihan {formatDays(review.excess)} hari. Saldo tidak
            berubah; catatan tersimpan di riwayat.
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody className="space-y-3">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="cr-note">
              Catatan
            </label>
            <Textarea
              id="cr-note"
              rows={3}
              value={note}
              onChange={(e) => {
                setNote(e.target.value);
                setServerError("");
              }}
              placeholder="Wajib diisi, mis. kelebihan dibahas dengan atasan dan dipotong dari gaji"
            />
            {tooLong ? <p className="mt-1 text-xs text-danger">Catatan maksimal {NOTE_MAX} karakter</p> : null}
          </div>
          {serverError ? (
            <p role="alert" className="rounded-lg bg-danger-soft px-3 py-2 text-sm text-danger">
              {serverError}
            </p>
          ) : null}
        </DialogPanelBody>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={resolve.isPending}>
            Batal
          </Button>
          <Button onClick={submit} disabled={trimmed === "" || tooLong || resolve.isPending} className="gap-2">
            {resolve.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Simpan
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}

/** Memuat saldo karyawan/tahun itu lalu membuka dialog ubah saldo yang sudah ada. */
function CorrectionDialog({ review, onClose }: { review: CarryReview; onClose: () => void }) {
  const balance = useLeaveBalance(review.employee_id, review.year);
  const failed = balance.isError;
  const message = balance.error instanceof Error ? balance.error.message : "";
  useEffect(() => {
    if (!failed) return;
    toast.error(message || "Gagal memuat saldo cuti");
    onClose();
  }, [failed, message, onClose]);
  if (!balance.data) return null;
  return (
    <LeaveBalanceEditDialog
      open
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
      balance={balance.data}
    />
  );
}

/**
 * Peringatan "Perlu ditinjau HR" (EPIC-068): sisa bawaan yang dihitung ulang
 * menjadi lebih kecil dari yang sudah terpakai. HR menandai selesai dengan
 * catatan wajib, atau mengoreksi saldo lewat dialog ubah. Tidak tampil bila
 * tidak ada tinjauan terbuka. `canUpdate` memberi aksi tandai selesai;
 * `canCorrect` (update + tahun berjalan/depan) memberi aksi Koreksi.
 */
export function LeaveCarryReviewsCard({
  year,
  canUpdate,
  canCorrect,
}: {
  year: number;
  canUpdate: boolean;
  canCorrect: boolean;
}) {
  const query = useCarryReviews(year);
  const [resolving, setResolving] = useState<CarryReview | null>(null);
  const [correcting, setCorrecting] = useState<CarryReview | null>(null);
  const reviews = query.data ?? [];
  if (query.isError) {
    return (
      <Card className="border-warning/40">
        <CardContent className="flex flex-wrap items-center justify-between gap-2 pt-6">
          <p role="alert" className="text-sm text-danger">
            Gagal memuat tinjauan sisa bawaan{query.error instanceof Error && query.error.message ? `: ${query.error.message}` : ""}.
          </p>
          <Button size="sm" variant="outline" onClick={() => void query.refetch()}>
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }
  if (query.isLoading) {
    return (
      <Card className="border-warning/40">
        <CardContent className="flex items-center gap-2 pt-6 text-sm text-gray-500" role="status">
          <Loader2 className="h-4 w-4 animate-spin" />
          Memuat tinjauan sisa bawaan…
        </CardContent>
      </Card>
    );
  }
  if (reviews.length === 0) return null;
  const hasActions = canUpdate || canCorrect;

  return (
    <Card className="border-warning/40">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <AlertTriangle className="h-5 w-5 text-warning" />
          Perlu ditinjau HR ({reviews.length})
        </CardTitle>
        <p className="text-sm text-gray-500">
          Sisa bawaan {year} dihitung ulang menjadi lebih kecil dari yang sudah terpakai. Saldo tetap tercatat apa
          adanya. Tandai selesai setelah dibahas, atau koreksi saldo karyawan.
        </p>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>NIP</TableHead>
                <TableHead>Nama</TableHead>
                <TableHead>Cabang</TableHead>
                <TableHead className="text-right">Bawaan</TableHead>
                <TableHead className="text-right">Terpakai</TableHead>
                <TableHead className="text-right">Kelebihan</TableHead>
                <TableHead>Penyebab</TableHead>
                <TableHead>Sejak</TableHead>
                {hasActions ? <TableHead className="w-48" /> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {reviews.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="text-gray-600">{r.nip ?? "-"}</TableCell>
                  <TableCell className="font-medium text-gray-900">{r.full_name}</TableCell>
                  <TableCell className="text-gray-600">{r.branch_name ?? "-"}</TableCell>
                  <TableCell className="text-right">{formatDays(r.carry_over_days)}</TableCell>
                  <TableCell className="text-right">{formatDays(r.carry_over_used)}</TableCell>
                  <TableCell className="text-right font-semibold text-warning">{formatDays(r.excess)}</TableCell>
                  <TableCell className="text-gray-600">{causeText(r)}</TableCell>
                  <TableCell className="text-gray-600">{formatDate(r.opened_at)}</TableCell>
                  {hasActions ? (
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        {canUpdate ? (
                          <Button size="sm" variant="outline" onClick={() => setResolving(r)}>
                            Tandai selesai
                          </Button>
                        ) : null}
                        {canCorrect ? (
                          <Button size="sm" variant="outline" onClick={() => setCorrecting(r)}>
                            Koreksi
                          </Button>
                        ) : null}
                      </div>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>

      {resolving ? <ResolveDialog review={resolving} onClose={() => setResolving(null)} /> : null}
      {correcting ? <CorrectionDialog review={correcting} onClose={() => setCorrecting(null)} /> : null}
    </Card>
  );
}
