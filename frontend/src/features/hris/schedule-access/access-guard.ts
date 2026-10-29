"use client";

import { createContext, useContext, useMemo } from "react";
import { ApiError } from "@/lib/api-client";

/** Teks kartu Jadwal Toko untuk akun yang bukan (lagi) pengatur jadwal toko. */
export const NOT_DELEGATE_MESSAGE = "Anda bukan pengatur jadwal toko. Hubungi HR bila seharusnya punya akses.";

/** 403 hak penempatan: penunjukan masih ada, pesannya sudah untuk pengguna. */
const NO_PLACE_RIGHT = "Anda tidak punya hak mengatur penempatan";

const isForbidden = (error: unknown): error is ApiError => error instanceof ApiError && error.status === 403;

/**
 * 403 lain dari API mode toko = penunjukan dicabut atau karyawannya dinonaktifkan
 * sementara halaman terbuka. Yang dikecualikan: wilayah ("Toko di luar wilayah
 * Anda", "N karyawan di luar wilayah Anda") dan hak penempatan.
 */
export function isDelegateAccessLost(error: unknown): boolean {
  return isForbidden(error) && error.message !== NO_PLACE_RIGHT && !error.message.endsWith("di luar wilayah Anda");
}

/** `retry` query mode toko: 403 tidak dicoba ulang, galat lain seperti bawaan aplikasi (2x). */
export function retryUnlessForbidden(failureCount: number, error: unknown): boolean {
  return !isForbidden(error) && failureCount < 2;
}

/** Dipasang halaman Jadwal Toko: dipanggil saat API mode toko menolak dengan 403. */
const StoreForbiddenContext = createContext<((lost: boolean) => void) | null>(null);
export const StoreForbiddenProvider = StoreForbiddenContext.Provider;

/**
 * Teks galat untuk komponen yang dipakai HR dan pengatur. Di luar halaman Jadwal
 * Toko sama dengan pesan server (atau `fallback`). Di dalamnya, 403 dilaporkan ke
 * halaman (akses dimuat ulang; akses hilang → kartu bukan pengatur) dan teks
 * server berbahasa Inggris tidak pernah tampil.
 */
export function useStoreErrorText() {
  const onForbidden = useContext(StoreForbiddenContext);
  return useMemo(() => {
    const lost = (error: unknown) => !!onForbidden && isDelegateAccessLost(error);
    const text = (error: unknown, fallback: string) =>
      lost(error)
        ? NOT_DELEGATE_MESSAGE
        : error instanceof Error && error.message
          ? error.message
          : fallback;
    const report = (error: unknown) => {
      if (onForbidden && isForbidden(error)) onForbidden(isDelegateAccessLost(error));
    };
    return {
      /** Galat ini berarti akun bukan pengatur lagi (hanya di halaman Jadwal Toko). */
      lost,
      /** Teks untuk ditampilkan saat render (tanpa efek samping). */
      text,
      /** Untuk efek query yang gagal. */
      report,
      /** Untuk callback galat (toast): laporkan lalu beri teksnya. */
      fail: (error: unknown, fallback: string) => {
        report(error);
        return text(error, fallback);
      },
    };
  }, [onForbidden]);
}
