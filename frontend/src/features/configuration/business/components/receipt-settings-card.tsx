"use client";

import { useEffect, useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { Loader2, ReceiptText } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  buildReceiptLines,
  type ReceiptPayload,
} from "@/components/pos/PrintReceipt";
import {
  RECEIPT_LINE_MAX_CHARS,
  RECEIPT_MAX_LINES_PER_SECTION,
  type PosReceiptSettings,
} from "@/lib/pos/receipt-settings";
import { toast } from "sonner";
import type { ReceiptStallOption } from "../api";
import { useReceiptSettings } from "../queries";
import { useSaveReceiptSettings } from "../mutations";

/**
 * EPIC-040 — Konfigurasi header/footer struk POS di Settings → Business.
 * Satu textarea per bagian (satu baris = satu baris struk); preview di kanan
 * memakai renderer struk ASLI (buildReceiptLines) dengan order contoh, jadi
 * preview = persis output print worker.
 */

const GLOBAL_SCOPE = "__global__";
const NO_ROWS: PosReceiptSettings[] = [];
const NO_STALLS: ReceiptStallOption[] = [];

type ScopeDraft = {
  headerText: string;
  footerText: string;
  showStallName: boolean;
};

const SAMPLE_PAYLOAD: ReceiptPayload = {
  orderNumber: "ORD-CONTOH-001",
  orderType: "dine_in",
  table: null,
  items: [
    { id: "1", productId: "p1", name: "Tsukune", price: 25000, quantity: 2 },
    {
      id: "2",
      productId: "p2",
      name: "Oolong Peach",
      price: 25000,
      quantity: 1,
    },
  ],
  notes: "",
  total: 75000,
  change: 25000,
  paymentMethod: "cash",
  discountAmount: 0,
  taxAmount: 0,
};

function linesToText(lines: string[]) {
  return lines.join("\n");
}

function textToLines(text: string) {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export function ReceiptSettingsCard() {
  const settingsQuery = useReceiptSettings();
  const saveMutation = useSaveReceiptSettings();
  const rows = settingsQuery.data?.data ?? NO_ROWS;
  const stalls = settingsQuery.data?.stalls ?? NO_STALLS;
  const loading = settingsQuery.isLoading;
  const saving = saveMutation.isPending;
  const [scope, setScope] = useState<string>(GLOBAL_SCOPE);
  // Isian per scope; tanpa suntingan memakai baris tersimpan scope itu (kosong bila
  // belum pernah diatur — fallback runtime tetap ke global, tampil sebagai placeholder).
  const [edits, setEdits] = useState<Record<string, ScopeDraft>>({});

  const saved = useMemo(() => {
    const row =
      scope === GLOBAL_SCOPE
        ? rows.find((r) => !r.branch_id && !r.warehouse_id)
        : rows.find((r) => r.warehouse_id === scope);
    return {
      headerText: linesToText(row?.header_lines ?? []),
      footerText: linesToText(row?.footer_lines ?? []),
      showStallName: row?.show_stall_name !== false,
    };
  }, [scope, rows]);
  const { headerText, footerText, showStallName } = edits[scope] ?? saved;
  const edit = (patch: Partial<ScopeDraft>) =>
    setEdits((prev) => ({
      ...prev,
      [scope]: { ...(prev[scope] ?? saved), ...patch },
    }));

  useEffect(() => {
    if (settingsQuery.isError) toast.error("Gagal memuat konfigurasi struk");
  }, [settingsQuery.isError]);

  const previewLines = useMemo(() => {
    return buildReceiptLines(
      {
        ...SAMPLE_PAYLOAD,
        receiptHeader: textToLines(headerText).map((l) =>
          l.slice(0, RECEIPT_LINE_MAX_CHARS),
        ),
        receiptFooter: textToLines(footerText).map((l) =>
          l.slice(0, RECEIPT_LINE_MAX_CHARS),
        ),
      },
      "CUSTOMER",
    );
  }, [headerText, footerText]);

  async function handleSave() {
    try {
      const stall = stalls.find((s) => s.id === scope);
      await saveMutation.mutateAsync({
        warehouse_id: stall?.id ?? null,
        branch_id: stall?.branch_id ?? null,
        header_lines: textToLines(headerText),
        footer_lines: textToLines(footerText),
        show_stall_name: showStallName,
      });
      setEdits((prev) => {
        const next = { ...prev };
        delete next[scope];
        return next;
      });
      toast.success("Konfigurasi struk tersimpan");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menyimpan");
    }
  }

  const textareaClass =
    "w-full rounded-md border border-gray-200 bg-white p-2 font-mono text-xs leading-5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/20";

  return (
    <div className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
      <div className="flex items-center gap-2">
        <ReceiptText className="h-4 w-4 text-brand-text" />
        <h3 className="text-sm font-semibold text-gray-800">
          Konfigurasi Struk
        </h3>
      </div>
      <p className="mt-0.5 text-xs text-gray-500">
        Header (identitas usaha) dan footer (ucapan penutup) struk kasir. Satu
        baris teks = satu baris struk; maks {RECEIPT_MAX_LINES_PER_SECTION}{" "}
        baris per bagian, {RECEIPT_LINE_MAX_CHARS} karakter per baris (kertas
        80mm). Berlaku juga untuk footer struk WhatsApp. Copy dapur/bar sengaja
        tidak memuatnya.
      </p>

      {loading ? (
        <div className="flex justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-gray-400" />
        </div>
      ) : (
        <div className="mt-4 grid gap-5 lg:grid-cols-2">
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">
                Scope
              </label>
              <select
                className="w-full rounded-md border border-gray-200 bg-white p-2 text-sm"
                value={scope}
                onChange={(e) => setScope(e.target.value)}
              >
                <option value={GLOBAL_SCOPE}>Global (semua outlet)</option>
                {stalls.map((stall) => (
                  <option key={stall.id} value={stall.id}>
                    {stallLabel(stall)}
                  </option>
                ))}
              </select>
              {scope !== GLOBAL_SCOPE && (
                <p className="mt-1 text-[11px] text-gray-400">
                  Outlet tanpa konfigurasi memakai konfigurasi Global.
                </p>
              )}
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">
                Header — nama usaha, alamat, kontak
              </label>
              <textarea
                rows={4}
                className={textareaClass}
                placeholder={
                  "PADDY STORE PVJ\nParis Van Java Mall, Jl. Sukajadi No. 131–139, Bandung\nIG @paddy.id"
                }
                value={headerText}
                onChange={(e) => edit({ headerText: e.target.value })}
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600">
                Footer — ucapan penutup, WiFi, promo
              </label>
              <textarea
                rows={3}
                className={textareaClass}
                placeholder={
                  "Terima kasih atas kunjungan Anda\nWiFi: PADDY-GUEST"
                }
                value={footerText}
                onChange={(e) => edit({ footerText: e.target.value })}
              />
            </div>
            <label className="flex items-center gap-2 text-xs text-gray-600">
              <input
                type="checkbox"
                checked={showStallName}
                onChange={(e) => edit({ showStallName: e.target.checked })}
              />
              Cetak baris &quot;Outlet: …&quot; di struk customer
            </label>
            <div className="flex justify-end">
              <Button onClick={handleSave} disabled={saving}>
                {saving ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  "Simpan Konfigurasi"
                )}
              </Button>
            </div>
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">
              Preview (renderer struk asli, order contoh)
            </label>
            <pre className="max-h-96 overflow-auto rounded-md border border-dashed border-gray-300 bg-gray-50 p-3 text-center font-mono text-[11px] leading-4 text-gray-700">
              {previewLines.join("\n")}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
}
