// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { ApiError } from "@/lib/api/auth";

const mocks = vi.hoisted(() => ({
  requireIamMenuPrefix: vi.fn(),
  query: vi.fn(),
  queryOne: vi.fn(),
  sendWhatsAppText: vi.fn(),
  sendEmail: vi.fn(),
}));

vi.mock("@/lib/api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: mocks.requireIamMenuPrefix,
}));
vi.mock("@/lib/db", () => ({ query: mocks.query, queryOne: mocks.queryOne }));
vi.mock("@/lib/whatsapp", () => ({ sendWhatsAppText: mocks.sendWhatsAppText }));
vi.mock("@/lib/resend", () => ({
  sendEmail: mocks.sendEmail,
  candidateStatusEmail: (name: string) => ({ subject: `Status ${name}`, html: "<p/>" }),
}));

import { POST } from "./route";

const send = (body: unknown) =>
  POST(
    new NextRequest("http://localhost/api/notifications/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
  );
const logRow = () => mocks.query.mock.calls[0][1] as unknown[];

beforeEach(() => {
  vi.clearAllMocks();
  mocks.requireIamMenuPrefix.mockResolvedValue({ id: "u1", role: "hrd", full_name: "HR", brand_id: null });
  mocks.queryOne.mockResolvedValue({ full_name: "Sari", email: "sari@x.id", phone: "+62 812-3456", status: "screening" });
  mocks.sendWhatsAppText.mockResolvedValue({ success: true });
  mocks.sendEmail.mockResolvedValue(true);
});

describe("POST /api/notifications/send", () => {
  it("401 tanpa sesi", async () => {
    mocks.requireIamMenuPrefix.mockRejectedValue(ApiError.unauthorized());
    expect((await send({ candidate_id: "c1" })).status).toBe(401);
  });

  it("403 untuk peran non-HR", async () => {
    mocks.requireIamMenuPrefix.mockResolvedValue({ id: "u1", role: "pos", full_name: "K", brand_id: null });
    expect((await send({ candidate_id: "c1" })).status).toBe(403);
    expect(mocks.queryOne).not.toHaveBeenCalled();
  });

  it("meminta menu Rekrutmen; HR tanpa menu itu ditolak 403", async () => {
    await send({ candidate_id: "c1" });
    expect(mocks.requireIamMenuPrefix).toHaveBeenCalledWith(["hris.recruitment"]);

    mocks.requireIamMenuPrefix.mockRejectedValue(ApiError.forbidden("Insufficient permissions"));
    mocks.queryOne.mockClear();
    mocks.sendWhatsAppText.mockClear();
    expect((await send({ candidate_id: "c1" })).status).toBe(403);
    expect(mocks.queryOne).not.toHaveBeenCalled();
    expect(mocks.sendWhatsAppText).not.toHaveBeenCalled();
  });

  it("404 bila kandidat tidak ada", async () => {
    mocks.queryOne.mockResolvedValue(null);
    expect((await send({ candidate_id: "c1" })).status).toBe(404);
  });

  it("WhatsApp default: nomor dibersihkan, pesan status, log 'sent'", async () => {
    const res = await send({ candidate_id: "c1" });
    expect(res.status).toBe(200);
    expect(mocks.sendWhatsAppText).toHaveBeenCalledWith({
      target: "628123456",
      message: "Halo Sari, Status lamaran kamu saat ini: screening",
    });
    expect(logRow()).toEqual(["c1", "whatsapp", "Halo Sari, Status lamaran kamu saat ini: screening", "sent", expect.any(String)]);
  });

  it("email gagal dicatat 'failed' dan dijawab 500", async () => {
    mocks.sendEmail.mockResolvedValue(false);
    const res = await send({ candidate_id: "c1", channel: "email" });
    expect(res.status).toBe(500);
    expect((await res.json()).error).toBe("Gagal mengirim email");
    expect(logRow()[3]).toBe("failed");
  });

  it("channel tidak dikenal ditolak 400", async () => {
    expect((await send({ candidate_id: "c1", channel: "sms" })).status).toBe(400);
  });
});
