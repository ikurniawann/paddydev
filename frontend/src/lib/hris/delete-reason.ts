/**
 * Alasan hapus HRIS (EPIC-061, Q-RA-4): setiap hapus/timpa data HRIS wajib
 * alasan ≥ 5 karakter. Satu dialog global (<DeleteReasonHost/>) dibuka lewat
 * askDeleteReason; mutasi hapus memakai withDeleteReason, alur jadwal memakai
 * retryWithReason (bertanya hanya bila server meminta).
 */

/** Sama persis dengan Go hrisdel.ErrReason. */
export const HRIS_DELETE_REASON_ERROR = "Alasan hapus wajib diisi (minimal 5 karakter)";
export const MIN_DELETE_REASON = 5;

export class DeleteCancelledError extends Error {
  constructor() {
    super("Penghapusan dibatalkan");
    this.name = "DeleteCancelledError";
  }
}

export interface DeleteReasonRequest {
  title: string;
  description?: string;
  confirmLabel?: string;
}

export interface PendingDeleteReason extends DeleteReasonRequest {
  id: number;
  resolve: (reason: string) => void;
  reject: (error: Error) => void;
}

let pending: PendingDeleteReason | null = null;
let nextId = 1;
const listeners = new Set<() => void>();

function publish(next: PendingDeleteReason | null) {
  pending = next;
  listeners.forEach((listener) => listener());
}

export function subscribeDeleteReason(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function currentDeleteReason(): PendingDeleteReason | null {
  return pending;
}

export function isValidDeleteReason(reason: string): boolean {
  return [...reason.trim()].length >= MIN_DELETE_REASON;
}

/** Buka dialog alasan; permintaan sebelumnya (bila ada) dibatalkan. */
export function askDeleteReason(request: DeleteReasonRequest): Promise<string> {
  pending?.reject(new DeleteCancelledError());
  return new Promise<string>((resolve, reject) => {
    const id = nextId++;
    const done = () => {
      if (pending?.id === id) publish(null);
    };
    publish({
      ...request,
      id,
      resolve: (reason) => {
        done();
        resolve(reason);
      },
      reject: (error) => {
        done();
        reject(error);
      },
    });
  });
}

export function withReasonQuery(url: string, reason: string): string {
  return `${url}${url.includes("?") ? "&" : "?"}reason=${encodeURIComponent(reason)}`;
}

/** mutationFn hapus: tanya alasan dulu, lalu panggil fn(arg, alasan). */
export function withDeleteReason<A, T>(title: string, fn: (arg: A, reason: string) => Promise<T>): (arg: A) => Promise<T> {
  return async (arg: A) => fn(arg, await askDeleteReason({ title }));
}

/** Jalankan run(); bila server meminta alasan, tanya lalu ulangi sekali. */
export async function retryWithReason<T>(title: string, run: (reason?: string) => Promise<T>): Promise<T> {
  try {
    return await run();
  } catch (error) {
    if (!(error instanceof Error) || error.message !== HRIS_DELETE_REASON_ERROR) throw error;
    return run(await askDeleteReason({ title }));
  }
}
