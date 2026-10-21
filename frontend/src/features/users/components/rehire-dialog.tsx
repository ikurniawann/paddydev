"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useStoreBranches } from "@/features/hris/placements/queries";
import { formatDate } from "@/lib/format";
import type { Employee } from "@/types/hris";
import { rehireEmployee, type RehireBody } from "../api";
import { useUserFormLookups } from "../queries";

export type RehireEmployee = Pick<Employee, "id" | "full_name" | "user_id" | "email" | "join_date" | "end_date">;

interface Props {
  open: boolean;
  employee: RehireEmployee;
  today: string;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

const PLACEHOLDER_EMAIL = /@(tanpa-email|resign)\.invalid$/i;
const NOT_ACTIVE_STATUSES = new Set(["resigned", "terminated"]);
const FIELD = "mt-1 block w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm";

/** YYYY-MM-DD + 1 hari (UTC, tanpa pergeseran zona waktu). */
function nextDay(date: string): string {
  const d = new Date(`${date.slice(0, 10)}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 1);
  return d.toISOString().slice(0, 10);
}

export function RehireDialog({ open, employee, today, onOpenChange, onDone }: Props) {
  const hasLogin = Boolean(employee.user_id);
  const branches = useStoreBranches();
  const lookups = useUserFormLookups();
  const statuses = (lookups.data?.employmentStatuses ?? []).filter((s) => !NOT_ACTIVE_STATUSES.has(s.code));

  // Penempatan memakai rentang inklusif: harus setelah tanggal keluar (atau tanggal masuk bila tak ada).
  const lastDay = (employee.end_date ?? employee.join_date).slice(0, 10);
  const minDate = nextDay(lastDay);
  const minLabel = employee.end_date ? "tanggal keluar" : "tanggal masuk sebelumnya";

  const [date, setDate] = useState(today);
  const [status, setStatus] = useState("");
  const [branchId, setBranchId] = useState("");
  const [reactivateLogin, setReactivateLogin] = useState(true);
  const [email, setEmail] = useState(() => (PLACEHOLDER_EMAIL.test(employee.email ?? "") ? "" : (employee.email ?? "")));
  const [reason, setReason] = useState("");
  const [saving, setSaving] = useState(false);

  const dateError = !date
    ? "Tanggal efektif wajib diisi"
    : date < minDate
      ? `Harus setelah ${minLabel} (${formatDate(lastDay)})`
      : date > today
        ? "Tidak boleh setelah hari ini"
        : null;
  const invalid = !!dateError || !status || !branchId;

  async function submit() {
    if (invalid) return;
    setSaving(true);
    try {
      const body: RehireBody = { effective_date: date, employment_status: status, branch_id: branchId };
      if (hasLogin) body.reactivate_login = reactivateLogin;
      else if (email.trim()) body.email = email.trim();
      if (reason.trim()) body.reason = reason.trim();
      const res = await rehireEmployee(employee.id, body);
      const note = res.data?.schedule_note;
      const message = res.message ?? "Karyawan diaktifkan kembali";
      if (note) toast.success(message, { description: note });
      else toast.success(message);
      onDone();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal mengaktifkan kembali karyawan");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Aktifkan kembali — {employee.full_name}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          <p className="rounded-lg bg-gray-50 p-3 text-gray-600">
            Data lama (NIP, riwayat, kontrak, absensi) tetap dipakai. Tanggal efektif menjadi tanggal masuk baru dan
            penempatan utama baru dibuat di cabang yang dipilih.
          </p>
          <label className="block">
            Tanggal efektif *
            <Input aria-label="Tanggal efektif *" type="date" min={minDate} max={today} value={date} onChange={(e) => setDate(e.target.value)} className="mt-1" />
            {dateError ? <span className="text-danger">{dateError}</span> : null}
          </label>
          <label className="block">
            Status kepegawaian *
            <select className={FIELD} value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="">{lookups.isLoading ? "Memuat status…" : "Pilih status"}</option>
              {statuses.map((s) => (
                <option key={s.code} value={s.code}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block">
            Cabang penempatan *
            <select className={FIELD} value={branchId} onChange={(e) => setBranchId(e.target.value)}>
              <option value="">{branches.isLoading ? "Memuat cabang…" : "Pilih cabang"}</option>
              {(branches.data ?? []).map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name} ({b.code})
                </option>
              ))}
            </select>
          </label>
          {hasLogin ? (
            <label className="flex items-start gap-2">
              <input type="checkbox" className="mt-0.5" checked={reactivateLogin} onChange={(e) => setReactivateLogin(e.target.checked)} />
              <span>Aktifkan kembali akun login</span>
            </label>
          ) : (
            <label className="block">
              Email
              <Input aria-label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="Opsional" className="mt-1" />
              <span className="text-xs text-gray-400">Kosongkan untuk mempertahankan email saat ini. Karyawan ini belum punya akun login.</span>
            </label>
          )}
          <label className="block">
            Alasan
            <textarea aria-label="Alasan" value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} className="mt-1 w-full rounded-lg border p-2" rows={3} />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button disabled={invalid || saving} onClick={submit}>Aktifkan kembali</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
