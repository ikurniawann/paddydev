"use client";

import { useQuery } from "@tanstack/react-query";
import { retryUnlessForbidden } from "@/features/hris/schedule-access/access-guard";
import { fetchAutoCheckoutOptions, fetchAutoCheckoutRules } from "./api";
import { autoCheckoutKeys } from "./query-keys";

export const useAutoCheckoutRules = () =>
  useQuery({ queryKey: autoCheckoutKeys.all, queryFn: fetchAutoCheckoutRules, retry: retryUnlessForbidden });

/** Pilihan dialog tambah; hanya diminta saat dialog dibuka. */
export const useAutoCheckoutOptions = (enabled: boolean) =>
  useQuery({
    queryKey: autoCheckoutKeys.options,
    queryFn: fetchAutoCheckoutOptions,
    enabled,
    retry: retryUnlessForbidden,
  });
