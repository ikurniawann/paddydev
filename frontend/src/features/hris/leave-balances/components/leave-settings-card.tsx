"use client";

import { useState } from "react";
import { Loader2, Repeat } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { formatDateTime } from "@/lib/format";
import { formatDays, isHalfStep, parseDays } from "../balance-math";
import { useSaveLeaveSettings } from "../mutations";
import { useLeaveSettings } from "../queries";

const MONTHS = [
  "Januari",
  "Februari",
  "Maret",
  "April",
  "Mei",
  "Juni",
  "Juli",
  "Agustus",
  "September",
  "Oktober",
  "November",
  "Desember",
];
// Tahun bukan kabisat: 29 Februari tidak dipakai sebagai tanggal kedaluwarsa.
const DAYS_IN_MONTH = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

interface Edits {
  enabled?: boolean;
  max?: string;
  month?: string;
  day?: string;
}

/** Galat isian maksimal: kosong = semua sisa dibawa (null). */
function maxProblem(text: string): { value: number | null; error: string | null } {
  if (text.trim() === "") return { value: null, error: null };
  const value = parseDays(text);
  if (value === null) return { value: null, error: "Batas harus berupa angka" };
  if (!isHalfStep(value)) return { value, error: "Batas harus kelipatan 0,5" };
  if (value < 0) return { value, error: "Batas tidak boleh negatif" };
  return { value, error: null };
}

function dateProblem(month: number, day: number): string | null {
  if (!Number.isInteger(month) || month < 1 || month > 12 || !Number.isInteger(day) || day < 1) {
    return "Tanggal kedaluwarsa tidak valid";
  }
  if (month === 2 && day === 29) return "Tanggal 29 Februari tidak dipakai";
  if (day > DAYS_IN_MONTH[month - 1]) return "Tanggal kedaluwarsa tidak valid";
  return null;
}

/**
 * Pengaturan sisa bawaan cuti (EPIC-068 R8/R9): aktif/nonaktif, batas hari, dan
 * tanggal kedaluwarsa. Tersimpan sebagai patch; batas kosong = bawa semua sisa.
 */
export function LeaveSettingsCard({ canUpdate }: { canUpdate: boolean }) {
  const query = useLeaveSettings();
  const save = useSaveLeaveSettings();
  const [edits, setEdits] = useState<Edits>({});
  const [serverError, setServerError] = useState("");
  const stored = query.data;

  const enabled = edits.enabled ?? stored?.carry_over_enabled ?? false;
  const locked = stored ? !stored.carry_over_ready : false;
  const maxText = edits.max ?? (stored?.carry_over_max_days != null ? String(stored.carry_over_max_days) : "");
  const monthText = edits.month ?? String(stored?.carry_over_expiry_month ?? 3);
  const dayText = edits.day ?? String(stored?.carry_over_expiry_day ?? 31);
  const patchEdits = (e: Edits) => {
    setEdits((prev) => ({ ...prev, ...e }));
    setServerError("");
  };

  const max = maxProblem(maxText);
  const month = Number(monthText);
  const day = dayText.trim() === "" ? NaN : Number(dayText);
  const dateError = dateProblem(month, day);
  const dirty =
    stored !== undefined &&
    (enabled !== stored.carry_over_enabled ||
      max.value !== stored.carry_over_max_days ||
      month !== stored.carry_over_expiry_month ||
      day !== stored.carry_over_expiry_day);
  const valid = !max.error && !dateError;

  function submit() {
    if (!valid) return;
    save.mutate(
      {
        carry_over_enabled: enabled,
        carry_over_max_days: max.value,
        carry_over_expiry_month: month,
        carry_over_expiry_day: day,
      },
      {
        onSuccess: (res) => {
          setEdits({});
          toast.success(res.message || "Pengaturan sisa cuti tersimpan");
        },
        onError: (err: Error) => setServerError(err.message || "Gagal menyimpan pengaturan"),
      }
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Repeat className="h-5 w-5 text-pink-600" />
          Sisa Bawaan Cuti
        </CardTitle>
        <div className="space-y-1 text-sm text-gray-500">
          <p>
            Bila aktif, sisa jatah tahun lalu dibawa ke tahun baru secara otomatis setiap 1 Januari (WIB), lalu berlaku
            sampai tanggal kedaluwarsa di bawah.
          </p>
          <p>Pengaktifan berlaku mulai 1 Januari berikutnya; perubahan di tengah tahun tidak menjalankan ulang tahun ini.</p>
          <p>
            Untuk tahun yang sudah berjalan, sisa bawaan diisi manual lewat tombol Ubah di daftar atau impor Excel.
          </p>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? (
          <Loader2 className="h-5 w-5 animate-spin text-gray-400" />
        ) : query.isError ? (
          <p className="text-sm text-danger">
            {query.error instanceof Error ? query.error.message : "Gagal memuat pengaturan"}
          </p>
        ) : (
          <>
            <label className="flex items-center gap-3 text-sm font-medium text-gray-700">
              <Switch
                checked={enabled}
                onCheckedChange={(v) => patchEdits({ enabled: v })}
                disabled={!canUpdate || (locked && !enabled)}
              />
              Bawa sisa cuti ke tahun berikutnya
            </label>
            {locked && (
              <p className="text-sm text-amber-700">
                Sisa bawaan belum bisa diaktifkan sampai perbaikan perhitungan ulang sisa bawaan terpasang.
              </p>
            )}

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="ls-max">
                  Maksimal hari yang dibawa
                </label>
                <Input
                  id="ls-max"
                  inputMode="decimal"
                  placeholder="Kosong = semua sisa"
                  value={maxText}
                  disabled={!canUpdate}
                  onChange={(e) => patchEdits({ max: e.target.value })}
                />
                {max.error ? (
                  <p className="mt-1 text-xs text-danger">{max.error}</p>
                ) : (
                  <p className="mt-1 text-xs text-gray-500">Kelipatan 0,5. Kosongkan untuk membawa semua sisa.</p>
                )}
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="ls-month">
                  Bulan kedaluwarsa
                </label>
                <select
                  id="ls-month"
                  value={monthText}
                  disabled={!canUpdate}
                  onChange={(e) => patchEdits({ month: e.target.value })}
                  className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-50"
                >
                  {MONTHS.map((name, i) => (
                    <option key={name} value={String(i + 1)}>
                      {name}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700" htmlFor="ls-day">
                  Tanggal kedaluwarsa
                </label>
                <Input
                  id="ls-day"
                  type="number"
                  min={1}
                  max={31}
                  value={dayText}
                  disabled={!canUpdate}
                  onChange={(e) => patchEdits({ day: e.target.value })}
                />
                {dateError ? <p className="mt-1 text-xs text-danger">{dateError}</p> : null}
              </div>
            </div>

            <p className="rounded-lg bg-gray-50 p-3 text-sm text-gray-600">
              {enabled
                ? `Aktif: ${max.value === null ? "semua sisa" : `sampai ${formatDays(max.value)} hari`} dibawa dan berlaku sampai ${
                    dateError ? "—" : `${day} ${MONTHS[month - 1]}`
                  } tahun berjalan.`
                : "Nonaktif: tidak ada sisa cuti yang dibawa otomatis."}
            </p>

            {serverError ? (
              <p role="alert" className="rounded-lg bg-danger-soft px-3 py-2 text-sm text-danger">
                {serverError}
              </p>
            ) : null}

            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs text-gray-400">
                {stored?.updated_by_name
                  ? `Terakhir diubah ${stored.updated_by_name} · ${formatDateTime(stored.updated_at)}`
                  : "Belum pernah diubah (default: nonaktif)"}
              </p>
              {canUpdate ? (
                <Button
                  onClick={submit}
                  disabled={!dirty || !valid || save.isPending}
                  className="gap-2 bg-pink-600 text-white hover:bg-pink-700"
                >
                  {save.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
                  Simpan pengaturan
                </Button>
              ) : null}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
