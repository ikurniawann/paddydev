"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { searchPlacements } from "@/features/hris/placements/api";
import { fetchDelegateSuggest, fetchScheduleDelegates } from "./api";
import { scheduleDelegateKeys } from "./query-keys";

export const useScheduleDelegates = () =>
  useQuery({ queryKey: scheduleDelegateKeys.all, queryFn: fetchScheduleDelegates });

/** Saran toko untuk karyawan yang baru dipilih; kosong/undefined = tidak diminta (mode ubah). */
export const useDelegateSuggest = (employeeId: string | undefined) =>
  useQuery({
    queryKey: scheduleDelegateKeys.suggest(employeeId ?? ""),
    queryFn: () => fetchDelegateSuggest(employeeId as string),
    enabled: !!employeeId,
    // Penempatan bisa berubah di antara dua pemilihan; selalu ambil saran terbaru.
    staleTime: 0,
    // Toko yang sudah dicentang HR tidak boleh berubah sendiri; gagal ditampilkan sebagai petunjuk.
    refetchOnWindowFocus: false,
    retry: false,
  });

/** Pencarian karyawan untuk pemilih (nama/NIP), 25 hasil teratas. */
export const useDelegateEmployeeSearch = (q: string, enabled = true) =>
  useQuery({
    queryKey: scheduleDelegateKeys.employees(q),
    queryFn: () => searchPlacements({ q: q || undefined, page: 1, page_size: 25 }),
    enabled,
    placeholderData: keepPreviousData,
  });
