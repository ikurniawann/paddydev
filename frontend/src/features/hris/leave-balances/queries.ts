"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  fetchCarryReviews,
  fetchLeaveBalance,
  fetchLeaveBalanceAccess,
  fetchLeaveBalances,
  fetchLeaveSettings,
} from "./api";
import { leaveBalanceQueryKeys } from "./query-keys";
import type { LeaveBalanceListParams } from "./types";

export const useLeaveBalances = (params: LeaveBalanceListParams) =>
  useQuery({
    queryKey: leaveBalanceQueryKeys.list(params),
    queryFn: () => fetchLeaveBalances(params),
    placeholderData: keepPreviousData,
  });

export const useLeaveBalance = (employeeId: string, year: number) =>
  useQuery({
    queryKey: leaveBalanceQueryKeys.one(employeeId, year),
    queryFn: () => fetchLeaveBalance(employeeId, year),
    enabled: !!employeeId,
  });

/**
 * Aksi Saldo Cuti akun ini. Gagal tutup: selama memuat atau bila error, `data`
 * kosong dan pemanggil memakai `data?.update === true` sehingga tombol tidak tampil.
 */
export const useLeaveBalanceAccess = () =>
  useQuery({
    queryKey: leaveBalanceQueryKeys.access(),
    queryFn: fetchLeaveBalanceAccess,
    staleTime: 60_000,
    retry: 1,
  });

/** `enabled` false = tanpa hak baca Saldo Cuti (mis. tab karyawan untuk akun tanpa aksi update). */
export const useLeaveSettings = (enabled = true) =>
  useQuery({ queryKey: leaveBalanceQueryKeys.settings(), queryFn: fetchLeaveSettings, enabled });

/** Tinjauan sisa bawaan yang masih terbuka pada tahun itu; `enabled` false = tanpa hak baca. */
export const useCarryReviews = (year: number, enabled = true) =>
  useQuery({
    queryKey: leaveBalanceQueryKeys.reviews(year),
    queryFn: () => fetchCarryReviews(year),
    enabled,
  });
