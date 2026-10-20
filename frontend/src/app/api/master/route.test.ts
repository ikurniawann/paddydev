// Master data HRIS: validasi body, pesan kode duplikat, dan tolak hapus yang masih dipakai.
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

const query = vi.fn();
const queryOne = vi.fn();

vi.mock("@/lib/db", () => ({
  query: (...a: unknown[]) => query(...a),
  queryOne: (...a: unknown[]) => queryOne(...a),
}));
vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: vi.fn().mockResolvedValue({ id: "u1", full_name: "HR", role: "hrd" }),
  requireIamAction: vi.fn().mockResolvedValue({ id: "u1", full_name: "HR", role: "hrd" }),
}));

const ID = "00000000-0000-4000-8000-000000000001";
const req = (body?: unknown) =>
  new Request("http://x/api", body === undefined ? {} : { method: "POST", body: JSON.stringify(body) }) as unknown as NextRequest;
const ctx = { params: Promise.resolve({ id: ID }) };

beforeEach(() => {
  query.mockReset().mockResolvedValue([]);
  queryOne.mockReset();
});

describe("/api/master/departments", () => {
  it("POST: kode di-uppercase, is_active default lewat COALESCE", async () => {
    queryOne.mockResolvedValue({ id: ID, code: "OPS" });
    const { POST } = await import("./departments/route");
    const res = await POST(req({ name: "Operasional", code: "ops" }));
    expect(res.status).toBe(201);
    expect(queryOne.mock.calls[0][1]).toEqual(["Operasional", "OPS", null, null]);
  });

  it("POST: kode duplikat jadi 400 berpesan spesifik", async () => {
    queryOne.mockRejectedValue(Object.assign(new Error("dup"), { code: "23505" }));
    const { POST } = await import("./departments/route");
    const res = await POST(req({ name: "Ops", code: "OPS" }));
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe("Kode departemen sudah digunakan");
  });

  it("DELETE: masih dipakai karyawan → 400 tanpa menghapus", async () => {
    queryOne.mockResolvedValue({ n: 3 });
    const { DELETE } = await import("./departments/[id]/route");
    const res = await DELETE(
      new Request("http://x/api?reason=Departemen%20dobel", { method: "DELETE" }) as unknown as NextRequest,
      ctx
    );
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe("Tidak dapat dihapus, masih ada 3 karyawan di departemen ini");
    expect(query).not.toHaveBeenCalled();
  });
});

describe("/api/master/departments hapus tanpa alasan", () => {
  it("DELETE: alasan kosong/pendek → 400 tanpa menyentuh database", async () => {
    const { DELETE } = await import("./departments/[id]/route");
    const res = await DELETE(new Request("http://x/api?reason=abcd", { method: "DELETE" }) as unknown as NextRequest, ctx);
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe("Alasan hapus wajib diisi (minimal 5 karakter)");
    expect(queryOne).not.toHaveBeenCalled();
    expect(query).not.toHaveBeenCalled();
  });
});

describe("/api/master/positions/[id]", () => {
  it("PUT: id bukan UUID → 400", async () => {
    const { PUT } = await import("./positions/[id]/route");
    const res = await PUT(req({ title: "Barista" }), { params: Promise.resolve({ id: "1 OR 1=1" }) });
    expect(res.status).toBe(400);
    expect(queryOne).not.toHaveBeenCalled();
  });
});
