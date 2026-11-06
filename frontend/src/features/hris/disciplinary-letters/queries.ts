"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchLetterLateness, fetchLetters, fetchMyLetters } from "./api";
import { letterKeys } from "./query-keys";
import type { LetterListParams } from "./types";

export const useLetters = (params: LetterListParams) =>
  useQuery({ queryKey: letterKeys.list(params), queryFn: () => fetchLetters(params) });

export const useMyLetters = () => useQuery({ queryKey: letterKeys.mine, queryFn: fetchMyLetters });

/** Keterlambatan periode berjalan karyawan terpilih; tidak diminta tanpa karyawan. */
export const useLetterLateness = (employeeId: string | undefined) =>
  useQuery({
    queryKey: letterKeys.lateness(employeeId ?? ""),
    queryFn: () => fetchLetterLateness(employeeId as string),
    enabled: !!employeeId,
    retry: false,
  });
