type QueryValue = string | number | boolean | undefined | null;

export function buildListUrl(base: string, params?: Record<string, QueryValue>) {
  if (!params) return base;

  const qs = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") {
      qs.set(key, String(value));
    }
  });

  const query = qs.toString();
  return query ? `${base}?${query}` : base;
}

/**
 * Error respons non-2xx. Membawa status, body mentah, dan `details` (bila ada)
 * supaya pemanggil bisa bereaksi pada kode error terstruktur (mis. geofence).
 */
export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;
  readonly details: unknown;
  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.details =
      body && typeof body === "object" && "details" in body ? (body as { details?: unknown }).details : undefined;
  }
}

async function parseJson<T>(res: Response): Promise<T> {
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    const raw =
      typeof json === "object" && json && "error" in json
        ? (json as { error?: unknown }).error
        : null;
    const message =
      typeof raw === "string" && raw.trim()
        ? raw
        : raw != null && typeof raw === "object" && "message" in raw
          ? String((raw as { message?: unknown }).message || `Request failed (${res.status})`)
          : `Request failed (${res.status})`;
    throw new ApiError(message, res.status, json);
  }
  return json as T;
}

export function apiGet<T>(url: string) {
  return fetch(url).then((res) => parseJson<T>(res));
}

export function apiPost<T>(url: string, body: unknown) {
  return fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then((res) => parseJson<T>(res));
}

export function apiPut<T>(url: string, body: unknown) {
  return fetch(url, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then((res) => parseJson<T>(res));
}

export function apiPatch<T>(url: string, body: unknown) {
  return fetch(url, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then((res) => parseJson<T>(res));
}

export function apiDelete(url: string) {
  return fetch(url, { method: "DELETE" }).then((res) => parseJson<{ success?: boolean }>(res));
}
