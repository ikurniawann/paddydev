"use client";

import { useEffect, useState } from "react";
import { BuildingStorefrontIcon } from "@heroicons/react/24/outline";
import { Check, Loader2, Store } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { POS_CHROME_BUTTON_CLASS } from "@/features/pos/components/pos-tablet-chrome-controls";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import {
  POS_CART_STORAGE_KEY,
  posCartHasItems,
} from "@/lib/pos/pos-sell-stall";
import { stallLabel, type StallLabelSource } from "@/lib/auth/stall-label";

type StallOption = StallLabelSource & { id: string; code: string };

/**
 * Tombol pindah stall di halaman kasir (permintaan owner 2026-08-16).
 *
 * Menumpang persis infrastruktur switcher yang sudah ada — GET
 * /api/auth/stall-options (daftar + stall aktif) dan POST
 * /api/auth/active-stall (ganti, izin diperiksa server) — komponen ini
 * murni pintu UI-nya di kasir.
 *
 * Dua aturan yang dijaga:
 * - User tanpa izin pindah stall (403 dari stall-options) TIDAK melihat
 *   tombol ini sama sekali — bukan tombol yang error saat diklik.
 * - Keranjang berisi memblokir perpindahan (aturan yang sama dengan
 *   CashierStallGate): katalog stall lain berbeda, item lama jadi tidak sah.
 */
export function StallSwitchButton() {
  const [allowed, setAllowed] = useState<boolean | null>(null);
  const [allAccess, setAllAccess] = useState(false);
  const [active, setActive] = useState<StallOption | null>(null);
  const [stalls, setStalls] = useState<StallOption[]>([]);
  const [open, setOpen] = useState(false);
  const [switchingId, setSwitchingId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch("/api/auth/stall-options");
        if (cancelled) return;
        if (!res.ok) {
          setAllowed(false);
          return;
        }
        const json = await res.json();
        // API kini 200 utk semua user login; izin pindah ada di can_switch.
        setAllowed(Boolean(json.data?.can_switch));
        setAllAccess(Boolean(json.data?.all_access));
        setActive(json.data?.active ?? null);
        setStalls(json.data?.stalls ?? []);
      } catch {
        if (!cancelled) setAllowed(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Tanpa izin / masih memuat / tidak ada pilihan lain → tidak ada tombol.
  if (!allowed || stalls.length === 0) return null;

  async function switchTo(stall: StallOption | null) {
    if (switchingId) return;
    if ((stall?.id ?? null) === (active?.id ?? null)) {
      setOpen(false);
      return;
    }
    try {
      if (posCartHasItems(localStorage.getItem(POS_CART_STORAGE_KEY))) {
        toast.error("Kosongkan atau selesaikan keranjang sebelum pindah outlet");
        return;
      }
    } catch {
      /* localStorage bisa tidak tersedia — biarkan server yang menolak */
    }

    setSwitchingId(stall?.id ?? "__all__");
    try {
      const res = await fetch("/api/auth/active-stall", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ warehouse_id: stall?.id ?? null }),
      });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(json.error ?? "Gagal pindah outlet");
      toast.success(`Pindah ke ${stall ? stallLabel(stall) : "Semua Outlet"} — memuat ulang…`);
      // Reload penuh, bukan invalidate query: katalog produk, billing, dan
      // gate stall semuanya diputuskan server per-request dari cookie.
      window.location.reload();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal pindah outlet");
      setSwitchingId(null);
    }
  }

  return (
    <>
      <Button
        type="button"
        variant="outline"
        onClick={() => setOpen(true)}
        className={POS_CHROME_BUTTON_CLASS}
      >
        <Store className="mr-2 h-4 w-4" />
        <span className="max-w-40 truncate">
          {active ? stallLabel(active) : allAccess ? "Semua Outlet" : "Pilih Outlet"}
        </span>
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogPanel size="sm">
          <DialogPanelHeader>
            <DialogPanelTitle>Pindah Outlet</DialogPanelTitle>
            <DialogPanelDescription>
              Kasir akan berpindah ke katalog & penjualan outlet yang dipilih.
            </DialogPanelDescription>
          </DialogPanelHeader>
          <DialogPanelBody>
            <ul className="max-h-72 space-y-2 overflow-y-auto pr-0.5 [scrollbar-width:thin]">
              {allAccess ? (
                <li>
                  <button
                    type="button"
                    disabled={switchingId !== null}
                    onClick={() => void switchTo(null)}
                    className={`flex w-full items-center gap-2.5 rounded-lg border px-3 py-2.5 text-left transition disabled:cursor-not-allowed disabled:opacity-60 ${
                      active === null
                        ? "border-primary/40 bg-primary/5"
                        : "border-gray-200/80 bg-white hover:border-primary/30 hover:bg-primary/5"
                    }`}
                  >
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-gray-200/70 bg-muted/50 text-muted-foreground">
                      {switchingId === "__all__" ? (
                        <Loader2 className="h-4 w-4 animate-spin text-brand-text" />
                      ) : (
                        <BuildingStorefrontIcon className="h-4 w-4" />
                      )}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-foreground">Semua Outlet</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        Katalog gabungan — 1 transaksi boleh lintas outlet
                      </span>
                    </span>
                    {active === null ? (
                      <span className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-brand-text">
                        <Check className="h-3.5 w-3.5" /> Aktif
                      </span>
                    ) : (
                      <span className="shrink-0 text-xs font-medium text-brand-text">Pilih</span>
                    )}
                  </button>
                </li>
              ) : null}
              {stalls.map((stall) => {
                const isActive = stall.id === active?.id;
                const busy = switchingId === stall.id;
                return (
                  <li key={stall.id}>
                    <button
                      type="button"
                      disabled={switchingId !== null}
                      onClick={() => void switchTo(stall)}
                      className={`flex w-full items-center gap-2.5 rounded-lg border px-3 py-2.5 text-left transition disabled:cursor-not-allowed disabled:opacity-60 ${
                        isActive
                          ? "border-primary/40 bg-primary/5"
                          : "border-gray-200/80 bg-white hover:border-primary/30 hover:bg-primary/5"
                      }`}
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
                      {isActive ? (
                        <span className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-brand-text">
                          <Check className="h-3.5 w-3.5" /> Aktif
                        </span>
                      ) : (
                        <span className="shrink-0 text-xs font-medium text-brand-text">
                          {busy ? "Pindah…" : "Pilih"}
                        </span>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          </DialogPanelBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
              className="border-border"
            >
              Tutup
            </Button>
          </DialogFooter>
        </DialogPanel>
      </Dialog>
    </>
  );
}
