"use client";

import { useState } from "react";
import Link from "next/link";
import { ChevronLeft, Loader2, Plus } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { PageHeader } from "@/components/ui/page-header";
import { formatDateTime } from "@/lib/format";
import { useRevokeDelegate } from "../mutations";
import { useScheduleDelegates } from "../queries";
import type { ScheduleDelegate } from "../types";
import { DelegateFormDialog } from "./delegate-form-dialog";

/** Chip toko yang ditampilkan per baris; sisanya diringkas "+n". */
const MAX_CHIPS = 3;

/**
 * HRIS → Penempatan Karyawan → Pengatur jadwal toko (EPIC-071): HR menunjuk
 * karyawan yang boleh mengatur jadwal (dan, bila dicentang, penempatan) di
 * toko-toko tertentu, mengubah wilayahnya, atau mencabut aksesnya.
 * `canManage` = aksi update di menu penempatan/kepegawaian (sama dengan guard
 * PUT/DELETE Go); tanpa itu halaman hanya bisa dibaca.
 */
export function ScheduleDelegatesPage({ canManage }: { canManage: boolean }) {
  const list = useScheduleDelegates();
  const revoke = useRevokeDelegate();
  const [editing, setEditing] = useState<ScheduleDelegate | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [revoking, setRevoking] = useState<ScheduleDelegate | null>(null);
  const rows = list.data ?? [];

  function openForm(delegate: ScheduleDelegate | null) {
    setEditing(delegate);
    setFormOpen(true);
  }

  function confirmRevoke() {
    if (!revoking) return;
    const target = revoking;
    revoke.mutate(target.employee_id, {
      onSuccess: () => {
        toast.success(`Akses ${target.full_name} dicabut`);
        setRevoking(null);
      },
      onError: (e: Error) => {
        toast.error(e.message || "Gagal mencabut akses");
        setRevoking(null);
      },
    });
  }

  return (
    <div className="space-y-5">
      <PageHeader
        kicker="HRIS · Master Data"
        title="Pengatur jadwal toko"
        description="Karyawan yang boleh mengatur jadwal kerja (dan penempatan, bila diizinkan) di toko yang Anda pilih. Aksesnya mengikuti orangnya, bukan jabatannya."
        actions={
          <>
            <Button variant="outline" asChild>
              <Link href="/dashboard/hris/placements">
                <ChevronLeft className="mr-1 h-4 w-4" /> Penempatan Karyawan
              </Link>
            </Button>
            {canManage ? (
              <Button onClick={() => openForm(null)}>
                <Plus className="mr-1.5 h-4 w-4" /> Tambah pengatur
              </Button>
            ) : null}
          </>
        }
      />

      <Card>
        <CardContent className="p-0">
          {list.isLoading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-gray-400" aria-label="Memuat" />
            </div>
          ) : list.isError ? (
            <div className="py-12 text-center text-sm text-red-500">Gagal memuat pengatur jadwal toko</div>
          ) : rows.length === 0 ? (
            <div className="py-12 text-center text-sm text-gray-400">
              Belum ada pengatur. Tambahkan karyawan yang boleh mengatur jadwal tokonya.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[880px] text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs text-gray-500">
                    <th className="min-w-[10rem] p-3">Karyawan</th>
                    <th className="min-w-[9rem] p-3">Jabatan</th>
                    <th className="min-w-[15rem] p-3">Toko</th>
                    <th className="p-3">Penempatan</th>
                    <th className="p-3">Bawahan</th>
                    <th className="p-3">Diubah</th>
                    {canManage ? <th className="p-3 text-right">Aksi</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <DelegateRow
                      key={row.employee_id}
                      row={row}
                      onEdit={canManage ? openForm : undefined}
                      onRevoke={canManage ? setRevoking : undefined}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {canManage ? <DelegateFormDialog open={formOpen} delegate={editing} onOpenChange={setFormOpen} /> : null}
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => !open && setRevoking(null)}
        title="Cabut akses pengatur?"
        description={
          revoking
            ? `${revoking.full_name} tidak lagi bisa mengatur jadwal di tokonya. Jadwal yang sudah dibuat tidak berubah.`
            : undefined
        }
        confirmLabel="Ya, cabut"
        loading={revoke.isPending}
        onConfirm={confirmRevoke}
      />
    </div>
  );
}

function DelegateRow({
  row,
  onEdit,
  onRevoke,
}: {
  row: ScheduleDelegate;
  /** Tanpa keduanya (hanya baca) kolom Aksi tidak ditampilkan. */
  onEdit?: (row: ScheduleDelegate) => void;
  onRevoke?: (row: ScheduleDelegate) => void;
}) {
  const shown = row.branches.slice(0, MAX_CHIPS);
  const more = row.branches.length - shown.length;
  return (
    <tr className="border-b border-gray-50 align-top last:border-0">
      <td className="p-3">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium text-gray-900">{row.full_name}</span>
          {!row.is_active && <Badge variant="muted">Nonaktif</Badge>}
        </div>
        <p className="text-xs text-gray-400">{row.nip ?? "—"}</p>
      </td>
      <td className="p-3 text-gray-700">{row.position_title ?? "—"}</td>
      <td className="p-3">
        <div className="flex flex-wrap gap-1">
          {shown.map((b) => (
            <span
              key={b.id}
              title={b.name}
              className="whitespace-nowrap rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-gray-700"
            >
              {b.code}
            </span>
          ))}
          {more > 0 && (
            <span
              title={row.branches.slice(MAX_CHIPS).map((b) => b.code).join(", ")}
              className="whitespace-nowrap rounded-full border border-gray-200 bg-white px-2 py-0.5 text-xs text-gray-500"
            >
              +{more}
            </span>
          )}
          {row.branches.length === 0 && <span className="text-gray-400">—</span>}
        </div>
      </td>
      <td className="p-3 text-gray-700">{row.can_place ? "Ya" : "Tidak"}</td>
      <td className="p-3 text-gray-700">{row.direct_reports}</td>
      <td className="p-3 text-xs text-gray-500">
        {row.updated_at ? <p className="whitespace-nowrap">{formatDateTime(row.updated_at)}</p> : null}
        {row.updated_by_name ? <p className="text-gray-400">{row.updated_by_name}</p> : null}
      </td>
      {onEdit && onRevoke ? (
        <td className="p-3">
          <div className="flex justify-end gap-2">
            <Button size="sm" variant="outline" onClick={() => onEdit(row)}>
              Ubah
            </Button>
            <Button size="sm" variant="outline" onClick={() => onRevoke(row)}>
              Cabut
            </Button>
          </div>
        </td>
      ) : null}
    </tr>
  );
}
