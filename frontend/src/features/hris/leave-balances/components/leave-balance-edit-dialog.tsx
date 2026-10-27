"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { todayWib } from "@/lib/dates";
import { formatDays, isHalfStep, parseDays, previewBalance } from "../balance-math";
import { useUpdateLeaveBalance } from "../mutations";
import type { LeaveBalanceView } from "../types";

interface FormState {
  total: string;
  adjustment: string;
  carryDays: string;
  expiresOn: string;
  reason: string;
}

const REASON_MAX = 500;

function initialState(b: LeaveBalanceView): FormState {
  return {
    total: String(b.annual_leave_total),
    adjustment: String(b.annual_leave_adjustment),
    carryDays: String(b.carry_over_days),
    expiresOn: b.carry_over_expires_on ?? "",
    reason: "",
  };
}

/** Satu isian angka: null bila kosong/bukan angka, dengan pesan galat bila ada. */
function figure(text: string, label: string, allowNegative: boolean): { value: number | null; error: string | null } {
  if (text.trim() === "") return { value: null, error: `${label} wajib diisi` };
  const value = parseDays(text);
  if (value === null) return { value: null, error: `${label} harus berupa angka` };
  if (!isHalfStep(value)) return { value, error: `${label} harus kelipatan 0,5` };
  if (!allowNegative && value < 0) return { value, error: `${label} tidak boleh negatif` };
  return { value, error: null };
}

/**
 * Ubah saldo cuti tahunan satu karyawan (EPIC-068 R11): jatah, penyesuaian,
 * sisa bawaan + kedaluwarsa, dan alasan wajib. "Terpakai" hanya dibaca; ia
 * berubah lewat persetujuan atau pembatalan cuti. Pratinjau sisa memakai
 * aturan yang sama dengan server (R4, tanggal hari ini WIB); pesan galat
 * server selalu ditampilkan apa adanya.
 */
export function LeaveBalanceEditDialog({
  open,
  onOpenChange,
  balance,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  balance: LeaveBalanceView;
}) {
  const [form, setForm] = useState<FormState>(() => initialState(balance));
  const [serverError, setServerError] = useState("");
  const update = useUpdateLeaveBalance();
  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
    setServerError("");
  };

  const total = figure(form.total, "Jatah", false);
  const adjustment = figure(form.adjustment, "Penyesuaian", true);
  const carry = figure(form.carryDays, "Sisa bawaan", false);
  const used = balance.annual_leave_used;
  const carryUsed = balance.carry_over_used;
  const reason = form.reason.trim();

  const preview = previewBalance({
    total: total.value ?? 0,
    adjustment: adjustment.value ?? 0,
    used,
    carryDays: carry.value ?? 0,
    carryUsed,
    expiresOn: form.expiresOn || null,
    today: todayWib(),
  });

  const errors: string[] = [total.error, adjustment.error, carry.error].filter((e): e is string => !!e);
  if (total.value !== null && adjustment.value !== null && preview.remaining < 0) {
    errors.push(
      `Sisa jatah tidak boleh kurang dari 0 (jatah ${formatDays(total.value)} + penyesuaian ${formatDays(adjustment.value)} − terpakai ${formatDays(used)} = ${formatDays(preview.remaining)})`
    );
  }
  // Only a changed carry-over (days or expiry) must cover what is used: a row the
  // server recalculated below its used days stays editable for other fields.
  const carryChanged = carry.value !== (balance.carry_over_days ?? 0) || (form.expiresOn || null) !== (balance.carry_over_expires_on ?? null);
  if (carry.value !== null && carryChanged && carry.value < carryUsed) {
    errors.push(`Sisa bawaan tidak boleh kurang dari yang sudah terpakai (${formatDays(carryUsed)})`);
  }
  if (reason.length > REASON_MAX) errors.push(`Alasan maksimal ${REASON_MAX} karakter`);
  const valid = errors.length === 0 && reason !== "";
  const noExpiryWarning = (carry.value ?? 0) > 0 && form.expiresOn === "";

  function submit() {
    if (!valid || total.value === null || adjustment.value === null || carry.value === null) return;
    update.mutate(
      {
        employeeId: balance.employee_id,
        payload: {
          year: balance.year,
          annual_leave_total: total.value,
          annual_leave_adjustment: adjustment.value,
          carry_over_days: carry.value,
          carry_over_expires_on: form.expiresOn || null,
          reason,
        },
      },
      {
        onSuccess: (res) => {
          toast.success(res.message || "Saldo cuti diperbarui");
          onOpenChange(false);
        },
        onError: (err: Error) => setServerError(err.message || "Gagal menyimpan saldo cuti"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="md">
        <DialogPanelHeader>
          <DialogPanelTitle>Ubah saldo cuti {balance.year}</DialogPanelTitle>
          <DialogPanelDescription>
            {balance.full_name}
            {balance.nip ? ` (${balance.nip})` : ""}. Perubahan dicatat di riwayat beserta alasannya.
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody className="space-y-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="lb-total">
                Jatah
              </label>
              <Input id="lb-total" inputMode="decimal" value={form.total} onChange={(e) => set("total", e.target.value)} />
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="lb-adjustment">
                Penyesuaian
              </label>
              <Input
                id="lb-adjustment"
                inputMode="decimal"
                value={form.adjustment}
                onChange={(e) => set("adjustment", e.target.value)}
              />
              <p className="mt-1 text-xs text-gray-500">Boleh negatif.</p>
            </div>
            <div>
              <p className="mb-1 text-sm font-medium text-gray-700">Terpakai</p>
              <p className="flex h-9 items-center rounded-md bg-gray-50 px-3 text-sm text-gray-700" data-testid="used">
                {formatDays(used)}
              </p>
              <p className="mt-1 text-xs text-gray-500">Berubah lewat persetujuan cuti.</p>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="lb-carry">
                Sisa bawaan
              </label>
              <Input
                id="lb-carry"
                inputMode="decimal"
                value={form.carryDays}
                onChange={(e) => set("carryDays", e.target.value)}
              />
              <p className="mt-1 text-xs text-gray-500">Sudah terpakai: {formatDays(carryUsed)}</p>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="lb-expires">
                Kedaluwarsa
              </label>
              <Input
                id="lb-expires"
                type="date"
                value={form.expiresOn}
                onChange={(e) => set("expiresOn", e.target.value)}
              />
              <p className="mt-1 text-xs text-gray-500">
                0 tanpa kedaluwarsa = dihitung otomatis; isi kedaluwarsa untuk menetapkan 0.
              </p>
            </div>
          </div>
          {noExpiryWarning ? (
            <p className="rounded-lg bg-warning-soft px-3 py-2 text-xs text-warning">
              Tanpa tanggal kedaluwarsa, sisa bawaan tidak berlaku dan tidak ikut sisa tersedia.
            </p>
          ) : null}

          <div className="grid grid-cols-2 gap-2 rounded-lg border border-dashed border-gray-200 bg-gray-50 p-3 text-sm">
            <div>
              <p className="text-xs uppercase tracking-wide text-gray-400">Sisa jatah</p>
              <p className="text-lg font-semibold text-gray-900" data-testid="preview-remaining">
                {formatDays(preview.remaining)}
              </p>
              <p className="text-xs text-gray-500">jatah + penyesuaian − terpakai</p>
            </div>
            <div>
              <p className="text-xs uppercase tracking-wide text-gray-400">Sisa tersedia hari ini</p>
              <p className="text-lg font-semibold text-gray-900" data-testid="preview-available">
                {formatDays(preview.available)}
              </p>
              <p className="text-xs text-gray-500">
                {preview.carryActive
                  ? `termasuk sisa bawaan ${formatDays(preview.carryRemaining)}`
                  : "tanpa sisa bawaan (tidak berlaku)"}
              </p>
            </div>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="lb-reason">
              Alasan
            </label>
            <Textarea
              id="lb-reason"
              rows={2}
              value={form.reason}
              onChange={(e) => set("reason", e.target.value)}
              placeholder="Wajib diisi, mis. koreksi jatah sesuai kontrak"
            />
          </div>

          {errors.length > 0 ? (
            <ul className="space-y-1 text-xs text-danger">
              {errors.map((e) => (
                <li key={e}>{e}</li>
              ))}
            </ul>
          ) : null}
          {serverError ? (
            <p role="alert" className="rounded-lg bg-danger-soft px-3 py-2 text-sm text-danger">
              {serverError}
            </p>
          ) : null}
        </DialogPanelBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={update.isPending}>
            Batal
          </Button>
          <Button onClick={submit} disabled={!valid || update.isPending} className="gap-2">
            {update.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Simpan
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}
