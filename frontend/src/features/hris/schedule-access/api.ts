import { apiGet } from "@/lib/api-client";
import type { ScheduleAccess } from "./types";

const EMPTY: ScheduleAccess = { is_delegate: false, can_place: false, branches: [] };

export const fetchScheduleAccess = () =>
  apiGet<{ data: ScheduleAccess }>("/api/hris/me/schedule-access").then((res) => ({
    ...EMPTY,
    ...res.data,
    branches: res.data?.branches ?? [],
  }));
