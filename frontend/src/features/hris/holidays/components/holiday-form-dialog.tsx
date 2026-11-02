"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { HolidayType } from "@/lib/hris/holidays";
import { HOLIDAY_TYPES } from "@/lib/hris/holidays-input";
import {
  emptyHolidayForm,
  holidayFormFrom,
  holidayPayload,
  withHolidayType,
  type HolidayStatus,
} from "@/lib/hris/holidays-view";
import { useSaveHoliday } from "../mutations";
import type { HolidayRow } from "../types";
import { HOLIDAY_TYPE_META } from "./holiday-badges";

interface HolidayFormDialogProps {
  open: boolean;
  holiday: HolidayRow | null;
  /** Tahun yang sedang dilihat; tanggal default form baru. */
  year: number;
  onOpenChange: (open: boolean) => void;
  /** Dipanggil dengan tahun tanggal yang disimpan supaya halaman bisa pindah tahun. */
  onSaved: (savedYear: number) => void;
}

export function HolidayFormDialog({ open, holiday, year, onOpenChange, onSaved }: HolidayFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{holiday ? "Edit Hari Libur" : "Tambah Hari Libur"}</DialogTitle>
        </DialogHeader>
        <HolidayFormBody
          key={holiday?.id ?? `new-${year}`}
          holiday={holiday}
          year={year}
          onCancel={() => onOpenChange(false)}
          onSaved={onSaved}
        />
      </DialogContent>
    </Dialog>
  );
}

interface HolidayFormBodyProps {
  holiday: HolidayRow | null;
  year: number;
  onCancel: () => void;
  onSaved: (savedYear: number) => void;
}

function HolidayFormBody({ holiday, year, onCancel, onSaved }: HolidayFormBodyProps) {
  const [form, setForm] = useState(() => (holiday ? holidayFormFrom(holiday) : emptyHolidayForm(year)));
  const save = useSaveHoliday();

  function handleSave() {
    if (!form.name.trim()) {
      toast.error("Nama libur wajib diisi");
      return;
    }
    save.mutate(
      { payload: holidayPayload(form), id: holiday?.id },
      {
        onSuccess: () => {
          toast.success(holiday ? "Hari libur diperbarui" : "Hari libur ditambahkan");
          onSaved(Number(form.holiday_date.slice(0, 4)));
        },
        onError: (error) => toast.error(error.message || "Gagal menyimpan"),
      }
    );
  }

  return (
    <>
      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Tanggal</label>
          <Input
            type="date"
            value={form.holiday_date}
            onChange={(e) => setForm((f) => ({ ...f, holiday_date: e.target.value }))}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Nama libur</label>
          <Input
            placeholder="cth. Hari Kemerdekaan Republik Indonesia"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">Tipe</label>
            <Select
              value={form.type}
              onValueChange={(v) => setForm((f) => withHolidayType(f, v as HolidayType))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {HOLIDAY_TYPES.map((type) => (
                  <SelectItem key={type} value={type}>
                    {HOLIDAY_TYPE_META[type].label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">Status</label>
            <Select
              value={form.status}
              onValueChange={(v) => setForm((f) => ({ ...f, status: v as HolidayStatus }))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="aktif">Aktif</SelectItem>
                <SelectItem value="draft">Draft (belum berlaku)</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <label className="flex items-start gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            className="mt-0.5"
            checked={form.deducts_leave}
            onChange={(e) => setForm((f) => ({ ...f, deducts_leave: e.target.checked }))}
          />
          <span>
            Tetap memotong jatah cuti tahunan
            <span className="block text-xs text-gray-400">
              Menurut SKB, cuti bersama memotong jatah cuti — libur nasional tidak.
            </span>
          </span>
        </label>
        {form.type === "cuti_bersama" && (
          <div className="flex items-start gap-3">
            <Switch
              aria-label="Ditetapkan libur oleh perusahaan"
              checked={form.is_day_off}
              onCheckedChange={(checked) => setForm((f) => ({ ...f, is_day_off: checked }))}
            />
            <span className="text-sm text-gray-700">
              Ditetapkan libur oleh perusahaan
              <span className="block text-xs text-gray-400">
                Karyawan HQ/Operations libur di tanggal ini. Bila mati, tanggal ini hari kerja biasa. Karyawan
                sistem shift mengikuti roster.
              </span>
            </span>
          </div>
        )}
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">Catatan (opsional)</label>
          <Input
            placeholder="cth. tanggal tentatif, verifikasi ke SKB resmi"
            value={form.note}
            onChange={(e) => setForm((f) => ({ ...f, note: e.target.value }))}
          />
        </div>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          Batal
        </Button>
        <Button onClick={handleSave} disabled={save.isPending}>
          {save.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Simpan"}
        </Button>
      </DialogFooter>
    </>
  );
}
