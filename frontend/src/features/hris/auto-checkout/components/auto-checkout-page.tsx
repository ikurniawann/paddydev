"use client";

import { useEffect, useState } from "react";
import { Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { PageHeader } from "@/components/ui/page-header";
import { Switch } from "@/components/ui/switch";
import { ApiError } from "@/lib/api-client";
import { useStoreErrorText } from "@/features/hris/schedule-access/access-guard";
import { useDeleteAutoCheckoutRule, useToggleAutoCheckoutRule } from "../mutations";
import { useAutoCheckoutRules } from "../queries";
import type { AutoCheckoutRule } from "../types";
import { AddRuleDialog } from "./add-rule-dialog";

export const AUTO_CHECKOUT_NOTE = "Absen karyawan yang belum clock-out ditutup otomatis di jam selesai shift.";

const ALL_LOCATIONS = "Semua lokasi";
const NO_DELETE_RIGHT = "Anda tidak punya hak menghapus aturan ini";

/** 403 "Insufficient permissions" dari DELETE: akun HR tanpa aksi hapus (pengatur toko tidak pernah kena). */
const isDeleteForbidden = (e: unknown) => e instanceof ApiError && e.status === 403 && e.message === "Insufficient permissions";

/**
 * Aturan checkout otomatis per jabatan dan lokasi (EPIC-080 R1/R2). HR
 * (Kepegawaian) mengatur semuanya; pengatur jadwal toko membuat/menyalakan/
 * menghapus aturan di tokonya dan hanya melihat aturan "semua lokasi".
 * `embedded` = dipakai sebagai tab di Jadwal Toko (tanpa judul halaman).
 */
export function AutoCheckoutPage({ embedded = false }: { embedded?: boolean }) {
  const list = useAutoCheckoutRules();
  const toggle = useToggleAutoCheckoutRule();
  const remove = useDeleteAutoCheckoutRule();
  const errorText = useStoreErrorText();
  const { report } = errorText;
  const [addOpen, setAddOpen] = useState(false);
  const [deleting, setDeleting] = useState<AutoCheckoutRule | null>(null);
  const rows = list.data ?? [];

  useEffect(() => {
    if (list.error) report(list.error);
  }, [list.error, report]);

  function handleToggle(rule: AutoCheckoutRule, isActive: boolean) {
    toggle.mutate(
      { id: rule.id, isActive },
      { onError: (e: Error) => toast.error(errorText.fail(e, "Gagal mengubah aturan")) }
    );
  }

  function confirmDelete() {
    if (!deleting) return;
    const target = deleting;
    remove.mutate(target.id, {
      onSuccess: () => {
        toast.success("Aturan dihapus");
        setDeleting(null);
      },
      onError: (e: Error) => {
        // HR tanpa aksi "delete" di Shift Kerja: 403 server (Inggris) jadi teks
        // sendiri, dan tidak dilaporkan sebagai "bukan pengatur" ke Jadwal Toko.
        toast.error(isDeleteForbidden(e) ? NO_DELETE_RIGHT : errorText.fail(e, "Gagal menghapus aturan"));
        setDeleting(null);
      },
    });
  }

  const addButton = (
    <Button onClick={() => setAddOpen(true)}>
      <Plus className="mr-1.5 h-4 w-4" /> Tambah aturan
    </Button>
  );

  return (
    <div className="space-y-5">
      {embedded ? (
        <div className="flex flex-wrap items-start justify-between gap-3">
          <p className="min-w-[min(16rem,100%)] flex-1 text-sm text-gray-500">{AUTO_CHECKOUT_NOTE}</p>
          {addButton}
        </div>
      ) : (
        <PageHeader
          kicker="HRIS · Kepegawaian"
          title="Checkout otomatis"
          description={AUTO_CHECKOUT_NOTE}
          actions={addButton}
        />
      )}

      <Card>
        <CardContent className="p-0">
          {list.isLoading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-gray-400" aria-label="Memuat" />
            </div>
          ) : list.isError ? (
            <div className="py-12 text-center text-sm text-red-500">{errorText.text(list.error, "Gagal memuat aturan")}</div>
          ) : rows.length === 0 ? (
            <div className="py-12 text-center text-sm text-gray-400">Belum ada aturan checkout otomatis.</div>
          ) : (
            <>
              <div className="hidden md:block" data-testid="rules-table">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs text-gray-500">
                      <th className="p-3">Jabatan</th>
                      <th className="p-3">Lokasi</th>
                      <th className="p-3">Aktif</th>
                      <th className="p-3 text-right">Hapus</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((rule) => (
                      <tr key={rule.id} className="border-b border-gray-50 last:border-0">
                        <td className="p-3 font-medium text-gray-900">{rule.position_title}</td>
                        <td className="p-3 text-gray-700">{rule.branch_name ?? ALL_LOCATIONS}</td>
                        <td className="p-3">
                          <RuleSwitch rule={rule} busy={toggle.isPending} onToggle={handleToggle} />
                        </td>
                        <td className="p-3 text-right">
                          <RuleDelete rule={rule} onDelete={setDeleting} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <ul className="divide-y divide-gray-100 md:hidden" data-testid="rules-cards">
                {rows.map((rule) => (
                  <li key={rule.id} className="flex items-center justify-between gap-3 p-4">
                    <div className="min-w-0">
                      <p className="break-words font-medium text-gray-900">{rule.position_title}</p>
                      <p className="break-words text-xs text-gray-500">{rule.branch_name ?? ALL_LOCATIONS}</p>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <RuleSwitch rule={rule} busy={toggle.isPending} onToggle={handleToggle} />
                      <RuleDelete rule={rule} onDelete={setDeleting} />
                    </div>
                  </li>
                ))}
              </ul>
            </>
          )}
        </CardContent>
      </Card>

      <AddRuleDialog open={addOpen} onOpenChange={setAddOpen} />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title="Hapus aturan?"
        description={
          deleting
            ? `${deleting.position_title} (${deleting.branch_name ?? ALL_LOCATIONS}) tidak lagi ditutup otomatis. Absen yang sudah ditutup tidak berubah.`
            : undefined
        }
        confirmLabel="Ya, hapus"
        loading={remove.isPending}
        onConfirm={confirmDelete}
      />
    </div>
  );
}

function RuleSwitch({
  rule,
  busy,
  onToggle,
}: {
  rule: AutoCheckoutRule;
  busy: boolean;
  onToggle: (rule: AutoCheckoutRule, isActive: boolean) => void;
}) {
  return (
    <Switch
      checked={rule.is_active}
      disabled={!rule.editable || busy}
      title={rule.editable ? undefined : "Hanya HR yang mengubah aturan semua lokasi atau toko lain"}
      aria-label={`${rule.is_active ? "Matikan" : "Nyalakan"} checkout otomatis ${rule.position_title} ${rule.branch_name ?? ALL_LOCATIONS}`}
      onCheckedChange={(checked) => onToggle(rule, checked)}
    />
  );
}

function RuleDelete({ rule, onDelete }: { rule: AutoCheckoutRule; onDelete: (rule: AutoCheckoutRule) => void }) {
  if (!rule.editable) return null;
  return (
    <Button
      size="sm"
      variant="outline"
      aria-label={`Hapus aturan ${rule.position_title} ${rule.branch_name ?? ALL_LOCATIONS}`}
      onClick={() => onDelete(rule)}
    >
      <Trash2 className="h-4 w-4" />
    </Button>
  );
}
