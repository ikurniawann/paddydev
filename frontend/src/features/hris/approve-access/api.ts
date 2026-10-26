import { apiGet } from "@/lib/api-client";
import type { HrApproveAccess } from "./types";

/** Hanya ada di Go (tanpa fallback Next); error dianggap tanpa akses oleh pemanggil. */
export const fetchHrApproveAccess = () =>
  apiGet<{ data: HrApproveAccess }>("/api/hris/me/approve-access").then((res) => res.data);
