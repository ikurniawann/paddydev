"use client";

import { useEffect, useState } from "react";
import { BuildingStorefrontIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { useConfirmAndSwitchStall, useCanUseCentralCashier } from "@/components/pos/confirm-stall-switch-dialog";
import { stallLabel, type StallLabelSource } from "@/lib/auth/stall-label";

type StallOption = StallLabelSource & { id: string; code: string };

function isAllStallMode(reason: string): boolean {
  return reason === "all" || reason === "all_stalls";
}

/**
 * Centered gate when kasir blocks "Semua Stall" / no assignment.
 * Stall list reuses the same APIs as sidebar StallSwitcher.
 * Central cashier in mode all is not blocked — overlay stays for regular users.
 */
export function CashierStallGate({ reason }: { reason: string }) {
  const canUseCentralCashier = useCanUseCentralCashier();
  const isNoStall = reason === "no_stall";
  const skipOverlay = canUseCentralCashier && isAllStallMode(reason);

  const title = isNoStall
    ? "Tidak ada outlet penempatan"
    : "Pilih outlet aktif untuk berjualan";
  const hint = isNoStall
    ? "Hubungi admin untuk assign outlet ke akun Anda."
    : "Mode Semua Outlet tidak diizinkan di kasir. Pilih satu outlet di bawah untuk mulai berjualan.";

  const [stalls, setStalls] = useState<StallOption[] | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [switchingId, setSwitchingId] = useState<string | null>(null);
  const { confirmAndSwitchStall, switching, dialogOpen, dialog } =
    useConfirmAndSwitchStall();
  const stallBusy = switching || dialogOpen;

  useEffect(() => {
    if (isNoStall || skipOverlay) return;
    let cancelled = false;

    async function loadStalls() {
      try {
        const res = await fetch("/api/auth/stall-options");
        const json = await res.json().catch(() => ({}));
        if (!res.ok) {
          throw new Error(json.error ?? "Gagal memuat daftar outlet");
        }
        if (cancelled) return;
        setStalls(
          ((json.data?.stalls ?? []) as StallOption[]).map(({ id, name, code, label }) => ({
            id,
            name,
            code,
            label,
          }))
        );
      } catch (error) {
        if (cancelled) return;
        setLoadFailed(true);
        setLoadError(error instanceof Error ? error.message : "Gagal memuat daftar outlet");
        setStalls([]);
      }
    }

    void loadStalls();
    return () => {
      cancelled = true;
    };
  }, [isNoStall, skipOverlay]);

  async function selectStall(warehouseId: string) {
    if (stallBusy) return;
    setSwitchingId(warehouseId);
    await confirmAndSwitchStall(warehouseId);
  }

  if (skipOverlay) return null;

  return (
    <div className="absolute inset-0 z-40 flex items-center justify-center px-4">
      {dialog}
      <div className="w-full max-w-lg rounded-xl border border-amber-200/80 bg-amber-50 px-4 py-4 text-center text-sm text-amber-950 shadow-sm">
        <p className="font-medium text-amber-950">{title}</p>
        <p className="mt-1 text-amber-900/90">{hint}</p>

        {!isNoStall && (
          <div className="mt-4 text-left">
            {stalls === null ? (
              <div className="flex items-center justify-center gap-2 py-4 text-amber-900/80">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>Memuat outlet…</span>
              </div>
            ) : loadFailed ? (
              <p className="rounded-lg border border-amber-200/80 bg-white/60 px-3 py-2 text-center text-xs text-amber-900/90">
                {loadError ?? "Gagal memuat daftar outlet"}. Ganti lewat switcher outlet di sidebar.
              </p>
            ) : stalls.length === 0 ? (
              <p className="rounded-lg border border-amber-200/80 bg-white/60 px-3 py-2 text-center text-xs text-amber-900/90">
                Tidak ada outlet yang bisa dipilih. Hubungi admin.
              </p>
            ) : (
              <ul className="max-h-56 space-y-2 overflow-y-auto pr-0.5 [scrollbar-width:thin]">
                {stalls.map((stall) => {
                  const busy = switchingId === stall.id && stallBusy;
                  const disabled = stallBusy;
                  return (
                    <li key={stall.id}>
                      <button
                        type="button"
                        disabled={disabled}
                        onClick={() => void selectStall(stall.id)}
                        className="flex w-full items-center gap-2.5 rounded-lg border border-amber-200/80 bg-white px-3 py-2.5 text-left transition hover:border-primary/30 hover:bg-primary/5 disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-gray-200/70 bg-muted/50 text-muted-foreground">
                          {busy ? (
                            <Loader2 className="h-4 w-4 animate-spin text-brand-text" />
                          ) : (
                            <BuildingStorefrontIcon className="h-4 w-4" />
                          )}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium text-foreground">
                            {stallLabel(stall)}
                          </span>
                          <span className="block truncate text-xs text-muted-foreground">
                            {stall.code}
                          </span>
                        </span>
                        <span className="shrink-0 text-xs font-medium text-brand-text">
                          {busy ? "Mengganti…" : "Pilih"}
                        </span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
