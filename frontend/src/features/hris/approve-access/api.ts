import { apiGet } from "@/lib/api-client";
import type { HrApproveAccess } from "./types";

/**
 * Hanya ada di Go (tanpa fallback Next). Handler membalas objek datar
 * `{attendance, leaves, overtime}` tanpa amplop `data`; error dianggap tanpa akses oleh pemanggil.
 */
export const fetchHrApproveAccess = () => apiGet<HrApproveAccess>("/api/hris/me/approve-access");
