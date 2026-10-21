"use client";

import { useQuery } from "@tanstack/react-query";

/** Data pilihan bersama halaman inventori (gudang, bahan baku). */

export type WarehouseLookup = {
  id: string;
  name: string;
  code: string;
  branch_name?: string;
  /** "<nama> — <branch>" bila daftar melintasi branch (tampilkan lewat stallLabel). */
  label?: string;
};
export type MaterialLookup = {
  id: string;
  kode: string;
  nama: string;
  satuan: string | null;
};

export async function fetchJson<T>(
  url: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(url, init);
  const body = await res.json().catch(() => ({}));
  if (!res.ok)
    throw new Error(body.message || body.error || "Permintaan gagal");
  return body as T;
}

export function useWarehouseLookup() {
  return useQuery({
    queryKey: ["inventory-lookup", "warehouses"],
    queryFn: async () =>
      (
        await fetchJson<{ data?: WarehouseLookup[] }>(
          "/api/purchasing/warehouses",
        )
      ).data ?? [],
    staleTime: 5 * 60_000,
  });
}

export function useMaterialLookup() {
  return useQuery({
    queryKey: ["inventory-lookup", "materials"],
    queryFn: async () =>
      (await fetchJson<{ data: MaterialLookup[] }>("/api/inventory/materials"))
        .data,
    staleTime: 5 * 60_000,
  });
}
