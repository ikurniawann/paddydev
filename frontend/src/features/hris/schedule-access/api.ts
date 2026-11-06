import { apiGet } from "@/lib/api-client";
import type { ScheduleAccess } from "./types";

const EMPTY: ScheduleAccess = {
  is_delegate: false,
  can_place: false,
  branches: [],
  schedule_all_branches: false,
  schedule_branches: [],
  hr: false,
  hr_can_place: false,
};

export const fetchScheduleAccess = () =>
  apiGet<{ data: ScheduleAccess }>("/api/hris/me/schedule-access").then((res) => ({
    ...EMPTY,
    ...res.data,
    branches: res.data?.branches ?? [],
    schedule_branches: res.data?.schedule_branches ?? res.data?.branches ?? [],
  }));
