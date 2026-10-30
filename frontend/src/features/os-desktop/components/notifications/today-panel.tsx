"use client";

import { X } from "lucide-react";
import type { DesktopOverview } from "@/lib/desktop/overview";
import { useDesktopInbox } from "../../hooks/use-desktop-inbox";

/**
 * Panel "Hari Ini" (⌘⇧J): ringkasan operasional + KOTAK KEPUTUSAN. Setiap
 * item membuka halaman yang tepat karena keputusannya butuh konteks penuh
 * (cuti: alasan & lampiran di halaman Cuti HR, EPIC-072 R6).
 */
export function TodayPanel({
  overview,
  onClose,
  onOpenPath,
}: {
  overview: DesktopOverview | null;
  onClose: () => void;
  onOpenPath: (path: string, title: string) => void;
}) {
  const inbox = useDesktopInbox();
  const sections = inbox.data ?? [];

  const pulsa = overview?.pulsaBisnis;
  const tim = overview?.timHariIni;
  const tamu = overview?.tamuDiMeja;
  const stats = [
    { label: "Omzet", value: pulsa ? `Rp ${Math.round(pulsa.hariIni.omzet).toLocaleString("id-ID")}` : "–" },
    { label: "Pesanan", value: pulsa ? String(pulsa.hariIni.pesanan) : "–" },
    { label: "Tamu duduk", value: tamu ? String(tamu.tamu) : "–" },
  ];

  return (
    <div className="fixed inset-0 z-[90] flex justify-end bg-black/30 backdrop-blur-sm" onClick={onClose}>
      <aside
        className="h-full w-[min(420px,100vw)] overflow-y-auto border-l border-white/12 bg-slate-950/90 p-5 text-white shadow-2xl backdrop-blur-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <div>
            <h2 className="text-base font-semibold">Hari Ini</h2>
            <p className="text-xs text-white/45">Ringkasan operasional & keputusan yang menunggu</p>
          </div>
          <button onClick={onClose} className="rounded-full p-2 text-white/55 transition hover:bg-white/10 hover:text-white">
            <X className="size-4" />
          </button>
        </div>

        <div className="grid grid-cols-3 gap-2">
          {stats.map((stat) => (
            <div key={stat.label} className="rounded-2xl border border-white/10 bg-white/6 p-3">
              <div className="text-[10px] uppercase tracking-wide text-white/45">{stat.label}</div>
              <div className="mt-1 truncate text-sm font-bold">{stat.value}</div>
            </div>
          ))}
        </div>
        {tim && (
          <div className="mt-2 rounded-2xl border border-white/10 bg-white/6 p-3 text-xs text-white/70">
            Tim: {tim.hadir} hadir · {tim.terlambat} terlambat · {tim.cuti} cuti
          </div>
        )}

        <h3 className="mb-2 mt-5 text-xs font-semibold uppercase tracking-[0.18em] text-white/40">Perlu keputusan</h3>
        {inbox.isPending && <div className="rounded-2xl bg-white/6 p-4 text-xs text-white/50">Memuat…</div>}
        {!inbox.isPending && sections.length === 0 && (
          <div className="rounded-2xl bg-white/6 p-4 text-xs text-white/50">Tidak ada yang menunggu keputusan Anda. 🎉</div>
        )}
        {sections.map((section) => (
          <div key={section.key} className="mb-3">
            <div className="mb-1.5 flex items-center justify-between text-xs">
              <span className="font-semibold text-white/75">{section.label}</span>
              <span className="rounded-full bg-white/10 px-2 py-0.5 text-[10px] font-bold">{section.total}</span>
            </div>
            <div className="space-y-1.5">
              {section.items.map((item) => (
                <div key={item.id} className="rounded-2xl border border-white/10 bg-white/6 p-3">
                  <div className="truncate text-sm font-semibold">{item.title}</div>
                  {item.subtitle && <div className="truncate text-[11px] text-white/45">{item.subtitle}</div>}
                  <div className="mt-2 flex gap-1.5">
                    <button
                      type="button"
                      onClick={() => onOpenPath(item.href, section.label)}
                      className="rounded-lg bg-white/10 px-3 py-1.5 text-[11px] font-semibold transition hover:bg-white/18"
                    >
                      Buka
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </aside>
    </div>
  );
}
