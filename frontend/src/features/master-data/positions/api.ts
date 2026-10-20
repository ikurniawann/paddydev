import { apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { PositionItem, PositionPayload } from "./types";

const BASE = "/api/master/positions";

export const fetchPositionList = () =>
  apiGet<{ data: PositionItem[] }>(BASE).then((res) => res.data);

export const createPosition = (body: PositionPayload) =>
  apiPost<{ data: PositionItem; message?: string }>(BASE, body);

export const updatePosition = (id: string, body: PositionPayload) =>
  apiPut<{ data: PositionItem; message?: string }>(`${BASE}/${id}`, body);

export const deletePosition = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
