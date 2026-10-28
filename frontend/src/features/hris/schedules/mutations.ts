"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { essQueryKeys } from "@/features/hris/ess/query-keys";
import { ApiError } from "@/lib/api-client";
import { DeleteCancelledError, isValidDeleteReason, retryWithReason } from "@/lib/hris/delete-reason";
import { saveScheduleCells } from "./api";
import { dedupeCells } from "./schedule-cells";
import type { CellEdit, CellsPreview } from "./types";

/** Batas server per simpan. */
export const CELLS_EMPLOYEE_LIMIT = 300;
export const CELLS_EMPLOYEE_LIMIT_MESSAGE = `Maksimal ${CELLS_EMPLOYEE_LIMIT} karyawan per simpan`;
export const CELLS_LIMIT = 6000;
export const CELLS_LIMIT_MESSAGE = `Maksimal ${CELLS_LIMIT} kotak per simpan — perkecil rentang tanggal atau jumlah karyawan`;

/** Jadwal ikut terlihat di Penempatan (status jadwal), profil karyawan, dan Shift Tim ESS. */
const AFFECTED_KEYS = [["hris", "schedules"], ["hris", "placements"], ["users", "shift-schedule"], essQueryKeys.team()] as const;

/** Muat ulang semua tampilan jadwal setelah jadwal tersimpan (per kotak, unggah, jadwal tetap). */
export function useInvalidateScheduleDependents() {
  const qc = useQueryClient();
  return () => Promise.all(AFFECTED_KEYS.map((queryKey) => qc.invalidateQueries({ queryKey: [...queryKey] })));
}

export interface CellsReview {
  cells: CellEdit[];
  preview: CellsPreview;
}

export function canCommit(preview: CellsPreview, ack: boolean, reason: string): boolean {
  return (
    preview.errors.length === 0 &&
    (preview.warnings.length === 0 || ack) &&
    (preview.replaced_days === 0 || isValidDeleteReason(reason))
  );
}

const message = (err: unknown, fallback: string) => (err instanceof Error && err.message ? err.message : fallback);

/**
 * Simpan kotak jadwal: pratinjau dulu. Tanpa perubahan (baru/ubah/kosongkan = 0)
 * tidak disimpan. Tanpa error/peringatan/timpaan langsung disimpan (kecuali
 * `confirm`); selain itu `review` terisi untuk dikonfirmasi (centang
 * peringatan, alasan bila ada jadwal yang tertimpa).
 */
export function useScheduleCellsSave(branchId: string, onSaved?: () => void) {
  const qc = useQueryClient();
  const invalidateDependents = useInvalidateScheduleDependents();
  const [review, setReview] = useState<CellsReview | null>(null);
  const [ack, setAck] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);

  function openReview(next: CellsReview | null) {
    setReview(next);
    setAck(false);
    setReason("");
  }

  async function commit(target: CellsReview, ackWarnings: boolean, givenReason: string): Promise<boolean> {
    setBusy(true);
    try {
      const res = await retryWithReason("Timpa jadwal yang sudah ada", (asked) =>
        saveScheduleCells({
          branch_id: branchId,
          step: "commit",
          fingerprint: target.preview.fingerprint,
          ack_warnings: ackWarnings,
          ...(givenReason.trim() || asked ? { reason: givenReason.trim() || asked } : {}),
          cells: target.cells,
        })
      );
      void invalidateDependents();
      toast.success(res.message ?? "Jadwal disimpan");
      openReview(null);
      onSaved?.();
      return true;
    } catch (err) {
      if (err instanceof DeleteCancelledError) return false;
      toast.error(message(err, "Gagal menyimpan jadwal"));
      if (err instanceof ApiError && err.status === 409) {
        openReview(null);
        void qc.invalidateQueries({ queryKey: ["hris", "schedules"] });
      }
      return false;
    } finally {
      setBusy(false);
    }
  }

  /** Pratinjau sel; `confirm` = selalu tampilkan ringkasan sebelum simpan (aksi massal). */
  async function save(input: CellEdit[], opts: { confirm?: boolean } = {}): Promise<void> {
    const cells = dedupeCells(input.filter((c) => c.code !== "-"));
    if (cells.length === 0) {
      toast.info("Tidak ada kotak yang perlu diubah");
      return;
    }
    if (new Set(cells.map((c) => c.employee_id)).size > CELLS_EMPLOYEE_LIMIT) {
      toast.error(CELLS_EMPLOYEE_LIMIT_MESSAGE);
      return;
    }
    if (cells.length > CELLS_LIMIT) {
      toast.error(CELLS_LIMIT_MESSAGE);
      return;
    }
    setBusy(true);
    let preview: CellsPreview;
    try {
      const res = await saveScheduleCells({ branch_id: branchId, step: "preview", cells });
      preview = { ...res.data, errors: res.data.errors ?? [], warnings: res.data.warnings ?? [], changes: res.data.changes ?? [] };
    } catch (err) {
      toast.error(message(err, "Jadwal tidak bisa diperiksa"));
      return;
    } finally {
      setBusy(false);
    }
    const { new: added, changed, cleared } = preview.summary;
    if (preview.errors.length === 0 && added + changed + cleared === 0) {
      toast.info("Tidak ada perubahan");
      return;
    }
    const target = { cells, preview };
    const clean = preview.errors.length === 0 && preview.warnings.length === 0 && !preview.replaced_days;
    if (clean && !opts.confirm) {
      await commit(target, false, "");
      return;
    }
    openReview(target);
  }

  return {
    busy,
    review,
    ack,
    setAck,
    reason,
    setReason,
    canSave: !!review && !busy && canCommit(review.preview, ack, reason),
    save,
    confirm: () => (review ? commit(review, ack, reason) : Promise.resolve(false)),
    cancel: () => openReview(null),
  };
}

export type ScheduleCellsSaver = ReturnType<typeof useScheduleCellsSave>;
