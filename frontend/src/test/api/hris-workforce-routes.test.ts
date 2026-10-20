/**
 * Perilaku route HRIS setelah migrasi ke apiHandler + zod: kode status dan
 * pesan galat Indonesia dipertahankan, aturan siklus kontrak, libur,
 * absensi, pengumuman, jadwal shift, task departemen tetap ditegakkan.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const requireIamMenuPrefix = vi.fn();
const requireIamGuard = vi.fn();
const getWorkforceActor = vi.fn();
const query = vi.fn();
const queryOne = vi.fn();
const withTransaction = vi.fn();
const single = vi.fn();

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args),
    // Aksi delete menu daun ikut grant palsu yang sama.
    requireIamAction: (prefixes: unknown) => requireIamMenuPrefix(prefixes),
    requireIamGuard: (...args: unknown[]) => requireIamGuard(...args),
  };
});
vi.mock("@/lib/hris/workforce-auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/hris/workforce-auth")>();
  return { ...actual, getWorkforceActor: () => getWorkforceActor() };
});
vi.mock("@/lib/db", () => ({
  query: (...args: unknown[]) => query(...args),
  queryOne: (...args: unknown[]) => queryOne(...args),
  withTransaction: (...args: unknown[]) => withTransaction(...args),
}));

/** Builder pg minimal: semua metode chain, hasil di single(). */
function fakeBuilder() {
  const builder: Record<string, unknown> = {};
  for (const method of ["from", "select", "eq", "update", "insert", "order", "range"]) {
    builder[method] = () => builder;
  }
  builder.single = () => single();
  return builder;
}
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: () => fakeBuilder(),
  createServerPgClient: async () => fakeBuilder(),
}));

const CONTRACT_ID = "00000000-0000-0000-0000-0000000000c1";
const EMPLOYEE_ID = "00000000-0000-0000-0000-0000000000e1";
const HR_USER = { id: "u-hr", full_name: "HR Satu", role: "hrd", brand_id: null };

function req(url: string, method = "GET", body?: unknown) {
  const request = new Request(url, {
    method,
    ...(body === undefined
      ? {}
      : { body: typeof body === "string" ? body : JSON.stringify(body), headers: { "Content-Type": "application/json" } }),
  }) as Request & { nextUrl: URL };
  request.nextUrl = new URL(url);
  return request as never;
}
const params = (values: { id: string }) => ({ params: Promise.resolve(values) });
const json = async (res: Response) => (await res.json()) as Record<string, unknown>;

const activePkwt = {
  id: CONTRACT_ID,
  employee_id: EMPLOYEE_ID,
  contract_number: "0001/PKWT/I/2026",
  contract_type: "pkwt",
  status: "active",
  start_date: "2026-01-01",
  end_date: "2026-12-31",
  probation_end_date: null,
  base_salary: "5000000",
  position_title: "Barista",
  department_name: "Ops",
  work_location: null,
  notes: null,
  signed_at: null,
  kemnaker_registered_at: null,
  compensation_paid_at: null,
  sequence: 1,
};

beforeEach(() => {
  for (const fn of [requireIamMenuPrefix, requireIamGuard, getWorkforceActor, query, queryOne, withTransaction, single]) {
    fn.mockReset();
  }
  requireIamMenuPrefix.mockResolvedValue(HR_USER);
  requireIamGuard.mockResolvedValue({ error: null, user: HR_USER });
  query.mockResolvedValue([]);
});

describe("PATCH/DELETE /api/hris/contracts/[id]", () => {
  const route = () => import("@/app/api/hris/contracts/[id]/route");
  const patch = (body: unknown, id = CONTRACT_ID) =>
    route().then(({ PATCH }) => PATCH(req("http://x/api/hris/contracts/" + id, "PATCH", body), params({ id })));

  it("ID bukan UUID → 400", async () => {
    const res = await patch({ action: "end" }, "abc");
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("ID kontrak tidak valid");
  });

  it("kontrak tidak ada → 404", async () => {
    queryOne.mockResolvedValueOnce(null);
    const res = await patch({ action: "end" });
    expect(res.status).toBe(404);
  });

  it("aksi tak dikenal → 400 dengan pesan lama", async () => {
    queryOne.mockResolvedValueOnce(activePkwt);
    const res = await patch({ action: "hapus" });
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Aksi tidak dikenal");
  });

  it("aktivasi kontrak yang sudah aktif → 409", async () => {
    queryOne.mockResolvedValueOnce(activePkwt);
    const res = await patch({ action: "activate" });
    expect(res.status).toBe(409);
    expect(withTransaction).not.toHaveBeenCalled();
  });

  it("pemutusan tanpa alasan → 400", async () => {
    queryOne.mockResolvedValueOnce(activePkwt);
    const res = await patch({ action: "terminate", reason: "  " });
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Alasan pemutusan kontrak wajib diisi");
  });

  it("path dokumen di luar folder kontrak ditolak", async () => {
    queryOne.mockResolvedValueOnce(activePkwt);
    const res = await patch({ action: "update", signed_document_url: "attendance/emp-x/a.jpg" });
    expect(res.status).toBe(400);
  });

  it("perpanjangan PKWT → 201, mulai sehari setelah kontrak lama", async () => {
    queryOne
      .mockResolvedValueOnce(activePkwt) // loadContract
      .mockResolvedValueOnce({ count: "3" }) // nomor kontrak berikutnya
      .mockResolvedValueOnce({ contract_number: "0004/PKWT/X/2026" }); // insert
    query.mockResolvedValueOnce([{ start_date: "2026-01-01", end_date: "2026-12-31" }]);
    const res = await patch({ action: "renew", end_date: "2027-06-30" });
    expect(res.status).toBe(201);
    expect((await json(res)).message).toContain("mulai 2027-01-01");
  });

  it("perpanjangan melewati batas 5 tahun → 422", async () => {
    queryOne.mockResolvedValueOnce(activePkwt);
    query.mockResolvedValueOnce([{ start_date: "2022-01-01", end_date: "2026-12-31" }]);
    const res = await patch({ action: "renew", end_date: "2027-12-31" });
    expect(res.status).toBe(422);
  });

  it("hapus kontrak non-draft → 409", async () => {
    queryOne.mockResolvedValueOnce(null);
    const { DELETE } = await route();
    const res = await DELETE(req("http://x?reason=Draft%20salah%20buat", "DELETE"), params({ id: CONTRACT_ID }));
    expect(res.status).toBe(409);
    expect((await json(res)).error).toBe("Hanya draft kontrak yang bisa dihapus");
  });
});

describe("POST /api/hris/holidays", () => {
  const post = (body: unknown) =>
    import("@/app/api/hris/holidays/route").then(({ POST }) => POST(req("http://x/api/hris/holidays", "POST", body)));

  it("tanggal salah → 400 dengan pesan lama", async () => {
    const res = await post({ holiday_date: "17-08-2026", name: "HUT RI" });
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Tanggal wajib diisi (YYYY-MM-DD)");
  });

  it("tanggal+nama kembar → 409", async () => {
    queryOne.mockRejectedValueOnce(Object.assign(new Error("dup"), { code: "23505" }));
    const res = await post({ holiday_date: "2026-08-17", name: "HUT RI" });
    expect(res.status).toBe(409);
    expect((await json(res)).error).toBe("Libur dengan tanggal dan nama yang sama sudah ada");
  });

  it("cuti bersama otomatis memotong cuti", async () => {
    queryOne.mockResolvedValueOnce({ id: "h1", name: "Cuti Bersama Idulfitri" });
    const res = await post({ holiday_date: "2026-03-23", name: "Cuti Bersama Idulfitri", type: "cuti_bersama" });
    expect(res.status).toBe(201);
    expect(queryOne.mock.calls[0][1][3]).toBe(true);
  });
});

describe("/api/hris/attendance", () => {
  const employee = { userId: "u-1", role: "employee", employeeId: "emp-me", isHr: false };

  it("action tak dikenal → 400", async () => {
    getWorkforceActor.mockResolvedValue(employee);
    const { POST } = await import("@/app/api/hris/attendance/route");
    const res = await POST(req("http://x/api/hris/attendance", "POST", { action: "pulang" }));
    expect(res.status).toBe(400);
  });

  it("clock-in tanpa selfie → 400 Validation failed", async () => {
    getWorkforceActor.mockResolvedValue(employee);
    const { POST } = await import("@/app/api/hris/attendance/route");
    const res = await POST(req("http://x/api/hris/attendance", "POST", { action: "clock-in" }));
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Validation failed");
  });

  it("karyawan tidak bisa clock-out absensi orang lain", async () => {
    getWorkforceActor.mockResolvedValue(employee);
    single.mockResolvedValueOnce({ data: { id: "a1", employee_id: "emp-lain", clock_out: null }, error: null });
    const { POST } = await import("@/app/api/hris/attendance/route");
    const res = await POST(
      req("http://x/api/hris/attendance", "POST", {
        action: "clock-out",
        attendance_id: "6f1c2a3b-4d5e-4f60-8a7b-9c0d1e2f3a4b",
        photo: "data:image/jpeg;base64,AAAA",
      })
    );
    expect(res.status).toBe(403);
  });

  it("detail absensi orang lain → 404 untuk karyawan", async () => {
    getWorkforceActor.mockResolvedValue(employee);
    single.mockResolvedValueOnce({ data: { id: "a1", employee_id: "emp-lain" }, error: null });
    const { GET } = await import("@/app/api/hris/attendance/[id]/route");
    const res = await GET(req("http://x"), params({ id: "a1" }));
    expect(res.status).toBe(404);
  });

  it("jadwal: akun non-HR tanpa record karyawan → 403", async () => {
    getWorkforceActor.mockResolvedValue({ ...employee, employeeId: null });
    const { GET } = await import("@/app/api/hris/attendance/schedule/route");
    const res = await GET(req("http://x/api/hris/attendance/schedule?employee_id=me"));
    expect(res.status).toBe(403);
    expect(query).not.toHaveBeenCalled();
  });
});

describe("POST /api/hris/announcements", () => {
  const post = (body: unknown) =>
    import("@/app/api/hris/announcements/route").then(({ POST }) =>
      POST(req("http://x/api/hris/announcements", "POST", body))
    );

  it("target departemen tanpa departemen → 400", async () => {
    getWorkforceActor.mockResolvedValue(null);
    const res = await post({ title: "Libur", target_scope: "department" });
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Pilih minimal satu departemen atau ubah target ke global");
    expect(withTransaction).not.toHaveBeenCalled();
  });

  it("URL video bukan YouTube/Vimeo → 400", async () => {
    getWorkforceActor.mockResolvedValue(null);
    const res = await post({ title: "Video", video_url: "https://contoh.test/v" });
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("URL video harus YouTube atau Vimeo yang valid");
  });

  it("judul kosong → 400 Validasi gagal", async () => {
    const res = await post({ title: "" });
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Validasi gagal");
  });
});

describe("validasi body route kepegawaian", () => {
  it("POST karyawan tanpa field wajib → 400", async () => {
    const { POST } = await import("@/app/api/hris/employees/route");
    const res = await POST(req("http://x/api/hris/employees", "POST", { full_name: "Budi" }));
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe(
      "Field yang wajib diisi: nama lengkap, email, tanggal bergabung, status karyawan"
    );
  });

  it("PUT jadwal shift kurang dari 7 hari → 400", async () => {
    const { PUT } = await import("@/app/api/hris/employees/[id]/shifts/route");
    const res = await PUT(
      req("http://x", "PUT", { effective_from: "2026-10-05", days: [{ day_of_week: 1, shift_id: null }] }),
      params({ id: EMPLOYEE_ID })
    );
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Pola jadwal harus lengkap 7 hari (Senin–Minggu)");
    expect(withTransaction).not.toHaveBeenCalled();
  });

  it("POST gaji tanpa gaji pokok → 400", async () => {
    const { POST } = await import("@/app/api/hris/employee-salary/route");
    const res = await POST(req("http://x", "POST", { employee_id: EMPLOYEE_ID }));
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Employee ID dan base salary wajib diisi");
  });

  it("POST riwayat kerja tanpa field wajib → 400", async () => {
    const { POST } = await import("@/app/api/hris/employment-history/route");
    const res = await POST(req("http://x", "POST", { employee_id: EMPLOYEE_ID }));
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Field wajib: employee_id, change_type, effective_date");
  });

  it("POST lowongan tanpa judul → 400", async () => {
    const { POST } = await import("@/app/api/hris/job-openings/route");
    const res = await POST(req("http://x", "POST", { status: "draft" }));
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Judul lowongan wajib diisi");
  });

  it("tolak feedback tanpa alasan → 400", async () => {
    const { POST } = await import("@/app/api/hris/feedback-approvals/reject/route");
    const res = await POST(req("http://x", "POST", { assignment_id: EMPLOYEE_ID, rejection_reason: " " }));
    expect(res.status).toBe(400);
    expect((await json(res)).error).toBe("Rejection reason is required");
  });

  it("body bukan JSON → 400, bukan 500", async () => {
    const { POST } = await import("@/app/api/hris/holidays/route");
    const res = await POST(req("http://x", "POST", "bukan-json"));
    expect(res.status).toBe(400);
  });
});

describe("POST /api/hris/dept-tasks", () => {
  it("karyawan tanpa bawahan tidak bisa membuat task → 403", async () => {
    getWorkforceActor.mockResolvedValue({ userId: "u-1", role: "employee", employeeId: EMPLOYEE_ID, isHr: false });
    queryOne.mockResolvedValueOnce({ department_id: "dept-1", full_name: "Budi", subordinates: 0 });
    const { POST } = await import("@/app/api/hris/dept-tasks/route");
    const res = await POST(req("http://x", "POST", { title: "Cek stok", recurrence: "daily" }));
    expect(res.status).toBe(403);
    expect((await json(res)).error).toBe(
      "Hanya HRD atau atasan (kepala tim) yang boleh membuat task departemen"
    );
  });
});
