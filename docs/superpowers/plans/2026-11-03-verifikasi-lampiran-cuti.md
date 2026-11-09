# Verifikasi Lampiran Cuti/Izin oleh HR (EPIC-088) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pengajuan cuti/izin berlampiran baru disetujui setelah atasan langsung menyetujui **dan** HR memverifikasi lampirannya. HR bisa mengembalikan lampiran agar karyawan mengunggah ulang.

**Architecture:**
- Status `hris.leaves` tetap `pending` selama menunggu HR. Tahap verifikasi dicatat di kolom baru `attachment_review` beserta kolom persetujuan atasan, sehingga kuota, bentrok, payroll, dan absensi tidak berubah.
- Go `hris`:
  - `DecideLeave` mengarahkan persetujuan atasan ke tahap `awaiting`;
  - route baru `POST /api/hris/leaves/attachment-review` untuk verify/return;
  - `UpdateLeave` menerima unggah ulang dari pemilik.
- Frontend: halaman HR Cuti & Izin, ESS Cuti, toast Persetujuan Tim, dan penjaga fallback Next.

**Tech Stack:** Go (hris, PostgreSQL delta), Next.js/TS (vitest).

**Spec:** `docs/epics/EPIC-088-verifikasi-lampiran-cuti.md` (R1–R10).

## Global Constraints
- Delta: `backend/database/migrations/deltas/20261109090000_leave_attachment_review.sql`. Kolom dan CHECK persis seperti R1.
- Verifikator = `a.CanApprove(iam.LeafCuti)`. Tidak ada yang memverifikasi pengajuannya sendiri.
- Pesan persis:
  - "Disetujui atasan — menunggu verifikasi lampiran oleh HR";
  - "Pengajuan sudah disetujui atasan dan menunggu verifikasi lampiran oleh HR";
  - "Pengajuan ini tidak menunggu verifikasi lampiran";
  - "Alasan pengembalian wajib diisi";
  - "Lampiran terverifikasi — cuti disetujui";
  - "Lampiran dikembalikan ke karyawan";
  - "Lampiran baru terkirim — menunggu verifikasi HR";
  - "Lampiran tidak valid";
  - "Pengajuan berlampiran diproses oleh layanan utama, coba lagi sebentar".
- Notifikasi:
  - HR: `NotifyHR(kind "leave_attachment", "Lampiran cuti menunggu verifikasi", …, "/dashboard/hris/leaves?review=awaiting")`;
  - karyawan: `NotifyEmployee(…, "leave_attachment", "Cuti disetujui" | "Lampiran cuti dikembalikan", …, "/dashboard/me/cuti")`.
- Audit `leave.attachment_verify` / `leave.attachment_return` (entity `hris.leaves`), memakai `auditDecision` seperti `DecideLeave`.
- Tes integrasi: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable'`, `go test -p 1`, `PATH=$HOME/.local/go/bin:$PATH`. Terapkan delta ke DB uji dengan `psql -f`. Setelah menambah route, jalankan `make -C backend generate`.
- Trailer commit `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit `.swarm`, `.claude-flow`, atau symlink `node_modules`.

## Review Focus
1. **HR yang juga atasan langsung** menyetujui pengajuan berlampiran bawahannya: langsung `approved` + `verified`, tidak terjebak di antreannya sendiri. Uji di Task 2.
2. **Cuti Tahunan berlampiran**: saldo terpotong tepat sekali, saat diverifikasi, bukan saat atasan setuju. Uji di Task 2.
3. **Unggah ulang dengan path karyawan lain** (`leave-attachments/<id lain>/x.png`): ditolak "Lampiran tidak valid". Uji di Task 2.
4. **Karyawan membatalkan saat menunggu HR atau dikembalikan**: berhasil, dan verifikasi sesudahnya → 400 "tidak menunggu". Uji di Task 2.
5. **Dua HR memverifikasi bersamaan**: satu sukses, satu 409; saldo tidak terpotong dua kali. Uji di Task 2 (verify kedua pada baris yang sama → 400/409, saldo tetap).

---

### Task 1: Kolom, aturan domain

**Files:**
- Create:
  - `backend/database/migrations/deltas/20261109090000_leave_attachment_review.sql`
  - `backend/internal/modules/hris/domain/leave_attachment_review.go`
- Test: `backend/internal/modules/hris/domain/leave_attachment_review_test.go`

**Interfaces:**
- Produces:
  - `func NeedsAttachmentReview(attachmentURL string, approverVerifies bool) bool`: true bila `strings.TrimSpace(attachmentURL) != ""` dan `!approverVerifies`;
  - `func LeaveAttachmentPathOK(path, employeeID string) bool`: path diawali `"leave-attachments/"+employeeID+"/"`, tidak mengandung `..`, dan punya nama file setelah prefix;
  - konstanta `AttachmentAwaiting = "awaiting"`, `AttachmentReturned = "returned"`, `AttachmentVerified = "verified"`.

- [ ] **Step 1: Failing tests**:
  - `NeedsAttachmentReview`: (`"leave-attachments/a/x.png"`, false) → true; (`"  "`, false) → false; (`"leave-attachments/a/x.png"`, true) → false.
  - `LeaveAttachmentPathOK`:
    - `("leave-attachments/E1/x.png","E1")` → true;
    - `("leave-attachments/E2/x.png","E1")` → false;
    - `("leave-attachments/E1/../E2/x.png","E1")` → false;
    - `("leave-attachments/E1/","E1")` → false;
    - `("","E1")` → false.
- [ ] **Step 2: Run** `go test ./internal/modules/hris/domain/ -run 'AttachmentReview|AttachmentPath'` (dari `backend/`). Expect FAIL.
- [ ] **Step 3: Implement** domain dan delta (R1, termasuk index parsial dan `COMMENT ON COLUMN hris.leaves.attachment_review`). Terapkan delta ke DB uji.
- [ ] **Step 4: Run.** Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): kolom verifikasi lampiran cuti + aturan domain (EPIC-088)`.

### Task 2: Alur backend — atasan, verifikasi HR, unggah ulang

**Files:**
- Modify:
  - `backend/internal/modules/hris/leaves.go` (`DecideLeave`, `UpdateLeave`, `leaveFilter`)
  - `postgres_leaves.go`
  - `approvals.go` (`TeamPendingLeaves`, `TeamPendingCount`)
  - `http_leaves.go`
  - `routes.go`
- Create: `backend/internal/modules/hris/leave_attachment_review.go` (service + handler)
- Test: `backend/internal/modules/hris/leave_attachment_review_integration_test.go`. Ikuti fixture `leave_approval_integration_test.go` (atasan, bawahan, HR pemegang `approve` Cuti).
- Regenerate: `frontend/src/lib/go-routes.generated.json`

**Interfaces:**
- Consumes: Task 1.
- Produces:
  - Repository:
    - `ManagerApproveLeave(ctx, id string, v leaveVerdict) error`: `UPDATE … SET attachment_review='awaiting', manager_approved_by, manager_approved_name, manager_approved_at=now(), decided_as WHERE id=$1 AND status='pending' AND attachment_review IS NULL`, `errNoRow` bila 0 baris;
    - `ReviewLeaveAttachment(ctx, id string, rv attachmentReview, credit *quotaMove) error`: `WHERE status='pending' AND attachment_review='awaiting'`. Untuk verify, status/approved_by/decided_* diisi sesuai R5 dan `consumeAnnualLeave` dipanggil bila `credit != nil`;
    - `ResubmitLeaveAttachment(ctx, id, path string) error`: `WHERE status='pending' AND attachment_review='returned'`;
    - `leaveFilter.Review *string`: hanya `awaiting`/`returned` yang dipakai, yaitu `status='pending' AND attachment_review=$n`.
  - Service: `func (s *Service) ReviewLeaveAttachment(ctx context.Context, a *Actor, in attachmentReviewInput, meta audit.Entry) (*Row, error)` dengan `attachmentReviewInput{LeaveID, Action string; Reason *string}`.
  - Route `POST /api/hris/leaves/attachment-review` (`h.withActor`). Respons `{ message, data }` dengan `data` = baris pengajuan, sama seperti `LeaveDecided`.
  - `DecideLeave` (R2–R4):
    - approve + `NeedsAttachmentReview(attachment_url, a.CanApprove(iam.LeafCuti))` → `ManagerApproveLeave` + `NotifyHR` dalam transaksi yang sama;
    - approve langsung oleh verifikator pada pengajuan berlampiran → `DecideLeave` lama, ditambah kolom `attachment_review='verified'` + pemeriksa (perluas `leaveVerdict` dengan `Verified bool`, `ReviewerID`, `ReviewerName`).
  - `TopicLeaveRequested`/WA tidak berubah. Respons `wa_link` R3 memakai kalimat "telah disetujui atasan dan menunggu verifikasi lampiran oleh HR".

- [ ] **Step 1: Failing tests** `TestLeaveAttachmentReviewFlow`:
  - Atasan menyetujui Birthday berlampiran → 200 dengan message R3. Baris: `status='pending'`, `attachment_review='awaiting'`, `manager_approved_by` = atasan. Ada 1 baris notifikasi HR berjudul "Lampiran cuti menunggu verifikasi". `GET /api/hris/me/team-approvals` atasan tidak lagi memuatnya, dan badge Persetujuan Tim turun 1.
  - Atasan approve lagi → 400 pesan R4. HR approve lewat `/leaves/approve` → 400 pesan R4. HR reject lewat `/leaves/approve` dengan alasan → 200 `rejected` (pada pengajuan terpisah).
  - Bukan verifikator (atasan) memanggil `attachment-review` → 403. Verifikator pada pengajuannya sendiri → 403.
  - `return` tanpa alasan → 400 "Alasan pengembalian wajib diisi". `return` dengan alasan → `attachment_review='returned'`, notifikasi karyawan "Lampiran cuti dikembalikan", satu audit `leave.attachment_return`.
  - Pemilik `PUT /api/hris/leaves/{id}` dengan path karyawan lain → 400 "Lampiran tidak valid" (Review Focus 3). Dengan path sendiri → 200 pesan R6, status `awaiting`, alasan dikosongkan.
  - `verify` → `status='approved'`, `approved_by` = atasan, `decided_as='manager'`, `attachment_review='verified'`, audit `leave.attachment_verify`, notifikasi "Cuti disetujui". `verify` kedua → 400 "Pengajuan ini tidak menunggu verifikasi lampiran" (Review Focus 5).
  - Cuti Tahunan berlampiran: `annual_leave_used` tidak berubah setelah atasan setuju, bertambah `total_days` setelah verify, tidak berubah lagi setelah verify kedua ditolak (Review Focus 2).
  - HR yang juga atasan langsung menyetujui → langsung `approved` + `verified` (Review Focus 1). Pengajuan tanpa lampiran disetujui atasan → langsung `approved`, `attachment_review` NULL.
  - Pemilik membatalkan saat `awaiting` → 200. `verify` sesudahnya → 400 (Review Focus 4).
  - `GET /api/hris/leaves?review=awaiting` (HR) hanya memuat pengajuan `awaiting`.
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/hris/ -run 'LeaveAttachmentReview'`. Expect FAIL.
- [ ] **Step 3: Implement.** Lalu `make -C backend generate`.
- [ ] **Step 4: Run** the test, then `go test -p 1 ./internal/modules/hris/ ./internal/app/`, `go vet ./...`, `gofmt -l internal`. Expect PASS and no gofmt output.
- [ ] **Step 5: Commit** `feat(hris): verifikasi lampiran cuti oleh HR setelah atasan + unggah ulang (EPIC-088)`.

### Task 3: Frontend + fallback Next + dokumen acuan

**Files:**
- Modify:
  - `frontend/src/features/hris/leaves/{types.ts,api.ts,mutations.ts}`: `LeaveItem` + `attachment_review`, `attachment_return_reason`, `manager_approved_name`, `attachment_reviewed_name`. `LeaveListParams.review`. Fungsi `reviewLeaveAttachment({leave_id, action, reason?})` dan `resubmitLeaveAttachment(id, path)`.
  - `features/hris/leaves/components/leaves-page.tsx`: opsi filter, badge, tombol Verifikasi/Kembalikan/Tolak, Detail (R8). Baca `?review=` dari URL sebagai filter awal.
  - `features/hris/ess/components/cuti-page.tsx` + `features/hris/ess/types.ts`: badge, alasan, "Unggah ulang lampiran".
  - `features/hris/team-approvals/components/persetujuan-tim-page.tsx`: toast = `message` server.
  - `frontend/src/lib/hris/leaves-repo.ts`: penjaga R9 di `decideLeave` (503).
  - `frontend/src/lib/backend-routes.test.ts`: `"/api/hris/leaves/attachment-review"` di `goNativePrefixes`.
  - `docs/hris/acuan-mom-20261019/status-requirement.md`: baris ES-11 → ✅ (EPIC-088).
  - `.claude/skills/paddy-hris-acuan/SKILL.md`: tambah baris "Lampiran cuti/izin | Atasan langsung lalu HR verifikasi lampiran; dikembalikan = unggah ulang (ES-11)".
- Test:
  - `features/hris/leaves/components/leaves-page.test.tsx` (tambah kasus)
  - `features/hris/ess/components/cuti-page.test.tsx`
  - `frontend/src/lib/hris/leaves-repo.test.ts` (atau tes fallback yang sudah ada untuk `decideLeave`)

**Interfaces:**
- Consumes: route dan kolom Task 2.
- Produces: label badge
  - `awaiting` → "Menunggu verifikasi HR";
  - `returned` → "Lampiran dikembalikan".

- [ ] **Step 1: Failing tests**:
  - Halaman HR, baris `pending` + `awaiting` milik orang lain (pengguna punya akses approve):
    - badge "Menunggu verifikasi HR";
    - tombol "Verifikasi" memanggil `reviewLeaveAttachment({leave_id, action:"verify"})`;
    - "Kembalikan" membuka dialog; tanpa alasan tombol kirim nonaktif; dengan alasan memanggil `action:"return", reason`;
    - tombol "Setujui" lama tidak tampil di baris itu.
  - ESS:
    - baris `returned` menampilkan "Lampiran dikembalikan", alasan, dan tombol "Unggah ulang lampiran";
    - setelah file dipilih, "Kirim" memanggil `resubmitLeaveAttachment`;
    - baris `awaiting` → "Menunggu verifikasi HR" tanpa tombol.
  - Fallback Next: `decideLeave` approve pada pengajuan ber-`attachment_url` → `ApiError` 503 dengan pesan R9; tanpa lampiran tetap jalan.
- [ ] **Step 2: Run** `npx vitest run src/features/hris/leaves src/features/hris/ess/components/cuti-page.test.tsx src/lib/hris/leaves-repo.test.ts src/lib/backend-routes.test.ts`. Expect FAIL.
- [ ] **Step 3: Implement.** Ikuti pola dialog alasan yang sudah ada untuk "Tolak" di `leaves-page.tsx`, dan `LeaveAttachmentField` untuk unggah ulang.
- [ ] **Step 4: Run** the tests, then:
  - `npx vitest run src/features/hris src/lib`;
  - `npx tsc --noEmit`;
  - `npx eslint --quiet` pada file yang berubah.

  Expect PASS.
- [ ] **Step 5: Commit** `feat(frontend): verifikasi lampiran cuti di HRIS dan unggah ulang di ESS (EPIC-088)`.

### Task 4: Review akhir, staging, PR

- [ ] Final review (reviewer terpisah, model paling mampu) → perbaikan.
- [ ] Backup staging, lalu deploy branch (migrasi + verify).
- [ ] QA dengan akun demo (dihapus sesudahnya; audit append-only tidak disentuh):
  - karyawan mengajukan Birthday + lampiran → atasan setuju → HR kembalikan → karyawan unggah ulang di HP → HR verifikasi → status Disetujui;
  - Cuti Tahunan berlampiran → saldo terpotong hanya setelah verifikasi;
  - pengajuan tanpa lampiran → langsung disetujui atasan;
  - notifikasi HR dan karyawan muncul.
- [ ] Docs: Automation Log epic, `docs/epics/README.md`.
- [ ] PR ke `main`. Merge dan production hanya dengan izin user; migrasi diterapkan sebelum binary baru.
