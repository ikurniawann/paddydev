import { apiDelete, apiGet, apiPatch, apiPost } from "@/lib/api-client";
import type { AutoCheckoutOptions, AutoCheckoutRule, AutoCheckoutRulePayload } from "./types";

const BASE = "/api/hris/auto-checkout-rules";

export const fetchAutoCheckoutRules = () =>
  apiGet<{ data: AutoCheckoutRule[] }>(BASE).then((res) => res.data ?? []);

export const fetchAutoCheckoutOptions = () =>
  apiGet<{ data: AutoCheckoutOptions }>(`${BASE}/options`).then((res) => ({
    positions: res.data?.positions ?? [],
    branches: res.data?.branches ?? [],
    all_locations: res.data?.all_locations ?? false,
  }));

export const createAutoCheckoutRule = (payload: AutoCheckoutRulePayload) =>
  apiPost<{ data: AutoCheckoutRule }>(BASE, payload).then((res) => res.data);

export const setAutoCheckoutRuleActive = (id: string, isActive: boolean) =>
  apiPatch<{ data: AutoCheckoutRule }>(`${BASE}/${id}`, { is_active: isActive }).then((res) => res.data);

export const deleteAutoCheckoutRule = (id: string) => apiDelete(`${BASE}/${id}`);
