"use client";

import Image from "next/image";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Loader2, Users } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EmployeeShiftsTab } from "@/features/users/components/employee-shifts-tab";
import { essQueryKeys } from "../query-keys";
import { useMyTeam } from "../queries";
import type { EssTeamMember } from "../types";

/**
 * Area Karyawan → Shift Tim (permintaan owner 2026-08-29): supervisor /
 * kepala divisi mengatur jadwal shift anggota TIM-NYA SENDIRI — bukan
 * hanya HRD. Anggota tim = bawahan langsung menurut employees.reporting_to;
 * server (/api/hris/me/team + guard di API jadwal) yang menegakkannya,
 * halaman ini hanya menampilkan.
 */

export function EssShiftTimPage() {
  const qc = useQueryClient();
  const teamQuery = useMyTeam();
  const members = teamQuery.data ?? null;
  const error = teamQuery.error
    ? teamQuery.error.message || "Gagal memuat anggota tim"
    : null;
  const [editing, setEditing] = useState<EssTeamMember | null>(null);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold text-gray-900">
          <CalendarDays className="h-6 w-6 text-brand-text" />
          Shift Tim
        </h1>
        <p className="mt-1 text-sm text-gray-500">
          Atur jadwal shift anggota tim Anda — perubahan langsung berlaku ke
          absensi, cuti, dan payroll mereka.
        </p>
      </div>

      {error ? (
        <Card>
          <CardContent className="p-6 text-sm text-red-600">{error}</CardContent>
        </Card>
      ) : members === null ? (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-sm text-gray-500">
            <Loader2 className="h-4 w-4 animate-spin" /> Memuat anggota tim…
          </CardContent>
        </Card>
      ) : members.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 p-10 text-center">
            <Users className="h-8 w-8 text-gray-300" />
            <p className="text-sm font-medium text-gray-700">
              Belum ada anggota tim yang tercatat di bawah Anda
            </p>
            <p className="max-w-md text-xs text-gray-500">
              Menu ini untuk supervisor / kepala divisi. Bila Anda seharusnya
              punya anggota tim, minta HRD melengkapi kolom atasan
              (reporting to) di data karyawan.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {members.map((member) => (
            <Card key={member.id}>
              <CardContent className="flex items-center gap-3 p-4">
                {member.photo_url ? (
                  <Image
                    src={member.photo_url}
                    alt={member.full_name}
                    width={40}
                    height={40}
                    unoptimized
                    className="h-10 w-10 rounded-full object-cover"
                  />
                ) : (
                  <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-brand-text">
                    {member.full_name.charAt(0).toUpperCase()}
                  </div>
                )}
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2 text-sm font-medium text-gray-900">
                    <span className="truncate">{member.full_name}</span>
                    {member.schedule_summary ? (
                      <span className="inline-flex flex-none items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 ring-1 ring-emerald-200">
                        ✓ Jadwal aktif
                      </span>
                    ) : (
                      <span className="inline-flex flex-none items-center rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-semibold text-amber-700 ring-1 ring-amber-200">
                        Belum ada jadwal
                      </span>
                    )}
                  </p>
                  <p className="truncate text-xs text-gray-500">
                    {[member.position_title, member.department_name]
                      .filter(Boolean)
                      .join(" · ") || "—"}
                  </p>
                  <p
                    className={`mt-0.5 truncate text-xs ${
                      member.schedule_summary ? "text-emerald-700" : "text-gray-400"
                    }`}
                  >
                    {member.schedule_summary
                      ? `${member.schedule_summary}${member.schedule_since ? ` · sejak ${member.schedule_since}` : ""}`
                      : "Atur pola shift supaya absensinya ternilai benar"}
                  </p>
                </div>
                {/* Sudah punya jadwal → tombol netral "Ubah Jadwal"; belum →
                    tombol utama "Atur Jadwal" supaya yang kosong menonjol. */}
                {member.schedule_type === "non_shift" ? (
                  <span className="flex-none rounded-full bg-info-soft px-2 py-0.5 text-[11px] font-semibold text-info">
                    Non-Shift — jadwal tetap
                  </span>
                ) : (
                  <Button
                    variant={member.schedule_summary ? "outline" : "default"}
                    size="sm"
                    onClick={() => setEditing(member)}
                  >
                    {member.schedule_summary ? "Ubah Jadwal" : "Atur Jadwal"}
                  </Button>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: essQueryKeys.team() }); // segarkan ringkasan jadwal
          }
        }}
      >
        {/* sm:max-w — bawaan DialogContent memasang sm:max-w-sm, jadi max-w
            tanpa prefix sm: kalah dan dialog menyusut (pilihan shift
            terpotong jadi "L."). */}
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>Jadwal Shift — {editing?.full_name}</DialogTitle>
          </DialogHeader>
          {editing ? <EmployeeShiftsTab employeeId={editing.id} /> : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
