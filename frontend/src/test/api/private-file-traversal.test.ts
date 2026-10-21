/**
 * Path traversal di route penyaji storage private (audit S1). Next men-decode
 * %2F di dalam satu segmen, jadi "emp-me" + "..%2F..%2Fleave-attachments"
 * dulu lolos cek segmen pertama/pemilik lalu membaca folder lain.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const readPrivateFile = vi.fn();
const getWorkforceActor = vi.fn();
const requireIamMenuPrefix = vi.fn();

vi.mock("@/lib/storage-private", () => ({
  readPrivateFile: (...args: unknown[]) => readPrivateFile(...args),
}));
vi.mock("@/lib/hris/workforce-auth", () => ({
  getWorkforceActor: () => getWorkforceActor(),
}));
vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return { ...actual, requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args) };
});

const call = async (
  load: () => Promise<{ GET: (req: never, ctx: { params: Promise<{ path: string[] }> }) => Promise<Response> }>,
  path: string[]
) => {
  const { GET } = await load();
  return GET(new Request("http://x") as never, { params: Promise.resolve({ path }) });
};

const attendancePhoto = () => import("@/app/api/hris/attendance/photo/[...path]/route");
const leaveAttachment = () => import("@/app/api/hris/leaves/attachment/[...path]/route");
const announcementCover = () => import("@/app/api/hris/announcements/cover/[...path]/route");
const psikotesFiles = () => import("@/app/api/psikotes/files/[...path]/route");
const interviewFiles = () => import("@/app/api/interview/files/[...path]/route");

beforeEach(() => {
  readPrivateFile.mockReset().mockResolvedValue({ data: Buffer.from("x"), mime: "image/jpeg" });
  getWorkforceActor.mockReset().mockResolvedValue({
    userId: "u-1",
    role: "employee",
    employeeId: "emp-me",
    isHr: false,
    isHrd: false,
  });
  requireIamMenuPrefix.mockReset().mockResolvedValue({ id: "hr-1", role: "hrd" });
});

describe("traversal ditolak sebelum membaca berkas", () => {
  it.each([
    ["attendance photo", attendancePhoto, ["attendance", "emp-me", "../../leave-attachments/emp-x/a.pdf"]],
    ["attendance photo (encoded)", attendancePhoto, ["attendance", "emp-me", "..%2F..%2Fcontracts", "a.pdf"]],
    ["leave attachment", leaveAttachment, ["leave-attachments", "emp-me", "..", "..", "contracts", "a.pdf"]],
    ["announcement cover", announcementCover, ["announcements", "../attendance/emp-x/a.jpg"]],
    ["psikotes file", psikotesFiles, ["psikotes", "..%2Fcontracts", "a.pdf"]],
    ["interview file", interviewFiles, ["interview", "..", "contracts", "a.pdf"]],
  ] as const)("%s", async (_name, load, path) => {
    const res = await call(load as never, [...path]);
    expect([400, 404]).toContain(res.status);
    expect(readPrivateFile).not.toHaveBeenCalled();
  });
});

describe("akses sah tetap jalan", () => {
  it("karyawan membaca selfie miliknya", async () => {
    const res = await call(attendancePhoto as never, ["attendance", "emp-me", "123-abc.jpg"]);
    expect(res.status).toBe(200);
    expect(readPrivateFile).toHaveBeenCalledWith("attendance/emp-me/123-abc.jpg");
  });

  it("karyawan lain tetap 403 (cek pemilik dipertahankan)", async () => {
    const res = await call(attendancePhoto as never, ["attendance", "emp-x", "123-abc.jpg"]);
    expect(res.status).toBe(403);
  });

  it("HR membaca berkas psikotes", async () => {
    const res = await call(psikotesFiles as never, ["psikotes", "sess-1", "draw.png"]);
    expect(res.status).toBe(200);
    expect(readPrivateFile).toHaveBeenCalledWith("psikotes/sess-1/draw.png");
  });
});
