"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchScheduleAccess } from "./api";
import { SCHEDULE_ACCESS_KEY } from "./query-keys";

/** Akses Jadwal Toko akun ini; `enabled` false = tidak diminta (mis. halaman dalam mode HR). */
export const useScheduleAccess = (enabled = true) =>
  useQuery({ queryKey: SCHEDULE_ACCESS_KEY, queryFn: fetchScheduleAccess, enabled });
