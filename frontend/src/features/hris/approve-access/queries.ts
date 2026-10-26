"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchHrApproveAccess } from "./api";
import { approveAccessQueryKeys } from "./query-keys";

/**
 * Akses `approve` per daun dari server (EPIC-067 R14). Gagal tutup: selama memuat
 * atau bila fetch error, `data` kosong sehingga pemanggil memakai `data?.x === true`
 * dan tab/tombol keputusan tidak tampil (tidak berkedip, tidak bocor).
 */
export const useHrApproveAccess = () =>
  useQuery({
    queryKey: approveAccessQueryKeys.all,
    queryFn: fetchHrApproveAccess,
    staleTime: 60_000,
    retry: 1,
  });
