"use client";

import { useEffect, useRef, useState } from "react";
import { AlarmClock, CheckCircle2, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useMyLateness } from "../queries";
import { LatenessDaysList } from "./lateness-days-list";
import { LatenessPeriodSelect } from "./lateness-period-select";
import { LatenessPolicy } from "./lateness-policy";
import { LatenessSummary } from "./lateness-summary";

/**
 * ESS → Absensi: kartu "Keterlambatan" (EPIC-077 R8) — jumlah terlambat
 * periode absensi terpilih, tingkat teguran/SP sesuai ketentuan, tangga
 * tingkat, hari terlambat, dan ketentuan perusahaan.
 */
export function MyLatenessCard({ hasSchedule = true }: { hasSchedule?: boolean }) {
  const [back, setBack] = useState(0);
  const query = useMyLateness(back);
  // Periode berjalan (query yang sama dengan Beranda): pemilih periode tetap ada
  // walau periode lain gagal dimuat.
  const current = useMyLateness(0).data;
  const data = query.data;
  const periods = (data ?? current)?.periods ?? [];
  const switching = query.isPlaceholderData;
  const ref = useRef<HTMLElement>(null);
  const scrolled = useRef(false);

  // Tautan Beranda "#keterlambatan": lompat sekali setelah data pertama tampil
  // (lompatan hash bawaan browser terjadi sebelum kartu ini ada).
  const loaded = Boolean(data);
  useEffect(() => {
    if (!loaded || scrolled.current) return;
    scrolled.current = true;
    if (window.location.hash === "#keterlambatan") ref.current?.scrollIntoView({ block: "start" });
  }, [loaded]);

  return (
    <section
      ref={ref}
      id="keterlambatan"
      aria-labelledby="keterlambatan-title"
      className="scroll-mt-24 space-y-4 rounded-2xl border border-gray-200/70 bg-white p-4 shadow-sm sm:p-5"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h2 id="keterlambatan-title" className="flex items-center gap-2 text-base font-semibold text-gray-900">
            <AlarmClock className="h-5 w-5 shrink-0 text-pink-600" />
            Keterlambatan
          </h2>
          <p className="mt-0.5 text-xs text-gray-500" aria-live="polite">
            {switching ? "Memuat periode…" : data ? `Periode absensi ${data.period.label}` : "Jumlah terlambat per periode absensi"}
          </p>
        </div>
        <LatenessPeriodSelect periods={periods} value={back} onChange={setBack} />
      </div>

      {query.isLoading ? (
        <div className="space-y-3" aria-label="Memuat keterlambatan">
          <Skeleton className="h-24 w-full rounded-2xl" />
          <Skeleton className="h-8 w-full" />
        </div>
      ) : query.isError || !data ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-red-100 bg-red-50 p-3 text-sm text-red-700">
          <span>Info keterlambatan gagal dimuat.</span>
          <Button size="sm" variant="outline" onClick={() => query.refetch()}>
            Coba lagi
          </Button>
        </div>
      ) : (
        <div className={cn("space-y-4 transition-opacity", switching && "opacity-50")} aria-busy={switching}>
          {/* EPIC-094: tingkat teguran/SP tidak ditampilkan ke karyawan. */}
          <LatenessSummary stats={data} showLevel={false} />

          {/* Tanpa jadwal tidak ada penilaian terlambat; hari terlambat lama (jadwal dicabut) tetap tampil. */}
          {!hasSchedule && data.count === 0 ? (
            <p className="flex items-start gap-2 rounded-xl bg-gray-50 p-3 text-xs text-gray-600">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-gray-400" />
              Jadwal shift belum diatur HR — keterlambatan belum dinilai.
            </p>
          ) : null}

          {data.days.length > 0 ? (
            <div className="space-y-2">
              <p className="text-sm font-semibold text-gray-800">Hari terlambat ({data.days.length})</p>
              <LatenessDaysList days={data.days} />
            </div>
          ) : (
            <p className="flex items-start gap-2 rounded-xl bg-emerald-50 p-3 text-sm text-emerald-800">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
              Tidak ada keterlambatan di periode ini. Pertahankan!
            </p>
          )}
        </div>
      )}

      <LatenessPolicy />
    </section>
  );
}
