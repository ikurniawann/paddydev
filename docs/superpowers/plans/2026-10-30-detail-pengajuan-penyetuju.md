# Detail Pengajuan untuk Penyetuju Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Atasan dan pemegang hak approve melihat alasan lengkap dan dokumentasi tiap pengajuan (cuti, lembur, pengajuan absensi, Perjalanan Dinas, kasbon) sebelum Setujui/Tolak.

**Architecture:** Satu dialog bersama `RequestDetailDialog` + pratinjau lampiran umum `AttachmentPreviewButtons`, dipasang sebagai tombol "Detail" di setiap antrean penyetuju (ESS dan HR). Server hanya dilonggarkan untuk pemegang `approve` daun Cuti/Lembur (daftar HR, detail cuti, file lampiran) dan antrean kasbon manager ikut mengirim `notes`.

**Tech Stack:** Next.js (frontend, Vitest + Testing Library), Go (`backend/internal/modules/{hris,payroll,desktop}`, tes integrasi Postgres).

**Spec:** `docs/epics/EPIC-072-detail-pengajuan-penyetuju.md` (Ruling R1–R9).

## Global Constraints
- Tanpa migrasi DB; tidak mengubah siapa yang boleh memutus (matriks approval tetap).
- Teks UI bahasa Indonesia; label tombol: "Detail", "Setujui", "Tolak", "Unduh".
- Alasan di dialog: `whitespace-pre-line break-words`, tidak dipotong.
- Dialog memakai `DialogPanel` + `DialogPanelHeader/Body` + `DialogFooter` (`components/ui/dialog.tsx`) supaya tidak meluber di HP (pelajaran PR #50).
- Path file privat di-encode per segmen saat membentuk URL.

## Review Focus
1. Pengajuan tanpa alasan / tanpa lampiran → dialog menampilkan "—" dan tidak merender bagian dokumentasi kosong.
2. Alasan sangat panjang dengan baris baru → dibungkus di dalam dialog, tidak memperlebar dialog (HP 390 px).
3. Lampiran PDF vs gambar → PDF dalam iframe, gambar `<img>`; nama file aneh (spasi) tetap terbuka (encode per segmen).
4. Penonton yang tidak berhak memutus (HR baca saja, pengajuan sendiri) → dialog tanpa tombol Setujui/Tolak.
5. Akun `approve` daun tanpa peran aplikasi HR → daftar HR terisi, lampiran 200; akun tanpa keduanya tetap 403 untuk lampiran orang lain.

---

### Task 1: Server — hak baca pemegang approve + `notes` kasbon + panel Hari Ini

**Files:**
- Modify: `backend/internal/modules/hris/leaves.go` (`ListLeaves`, `Leave`), `backend/internal/modules/hris/overtime.go` (`ListOvertime`), `backend/internal/modules/hris/http_files.go` (`leaveAttachment`, `overtimeAttachment`, `managerReadableFile`)
- Modify: `backend/internal/modules/payroll/loans.go` (`listLoanApprovals` kolom `notes`)
- Modify: `backend/internal/modules/desktop/postgres.go` + domain (cuti: subtitle memuat alasan singkat, `Href = "/dashboard/hris/leaves"`, `Actionable = false`)
- Test: tes integrasi baru `backend/internal/modules/hris/approver_read_integration_test.go`; sesuaikan tes desktop bila ada yang memeriksa href/actionable.

**Interfaces:**
- Produces: `func (a *Actor) hrReads(leaf string) bool { return a.IsHR || a.CanApprove(leaf) }` di `service.go`.
- `managerReadableFile(w, r, a, prefix, folder, leaf string)`: staff = `a.hrReads(leaf)`; atasan langsung tetap.

- [ ] Tes integrasi gagal dulu: akun peran aplikasi non-HR dengan role IAM yang memberi `approve` di `hris.workforce.leaves` dan `hris.kepegawaian.overtime` → `GET /api/hris/leaves` memuat cuti karyawan lain; `GET /api/hris/overtime` memuat lembur karyawan lain; `GET /api/hris/leaves/{id}` 200; `GET /api/hris/leaves/attachment/...` dan `/overtime/attachment/...` milik karyawan lain 200. Akun tanpa approve: lampiran orang lain 403.
- [ ] Implementasi `hrReads` dan pakai di keempat titik.
- [ ] `listLoanApprovals` SELECT tambah `notes`.
- [ ] Panel Hari Ini cuti: tanpa aksi langsung, href benar, alasan maks. 80 karakter di subtitle.
- [ ] `go vet` + `go test ./internal/modules/hris/... ./internal/modules/payroll/... ./internal/modules/desktop/...` hijau. Commit.

### Task 2: Komponen bersama

**Files:**
- Create: `frontend/src/components/hris/attachment-preview.tsx` (`AttachmentPreviewButtons`)
- Modify: `frontend/src/components/hris/overtime-attachment-preview.tsx` → `OvertimeAttachmentButtons` membungkus `AttachmentPreviewButtons` dengan `hrefFor={overtimeAttachmentHref}`
- Modify: `frontend/src/lib/hris/` — ekspor `leaveAttachmentHref(path)` (pindahan dari `persetujuan-tim-page.tsx`), `attendance-requests/api.ts` `attendanceRequestAttachmentUrl` encode per segmen
- Create: `frontend/src/components/hris/request-detail-dialog.tsx` + test

**Interfaces (Produces):**
```ts
AttachmentPreviewButtons(props: { paths: string[]; hrefFor: (path: string) => string; label?: string /* default "Dokumentasi" */; className?: string; buttonClassName?: string })

interface RequestDetailField { label: string; value: React.ReactNode }
RequestDetailDialog(props: {
  open: boolean; onOpenChange: (open: boolean) => void;
  title: string;                       // mis. "Detail cuti"
  employee: { name: string; nip?: string | null; subtitle?: string | null };
  status?: React.ReactNode;
  fields: RequestDetailField[];
  reason?: { label?: string /* default "Alasan" */; text: string | null | undefined };
  attachments?: { paths: string[]; hrefFor: (p: string) => string; label?: string };
  notes?: React.ReactNode;             // info tambahan (keputusan, catatan)
  actions?: React.ReactNode;           // tombol kaki; tanpa ini hanya "Tutup"
})
```
- [ ] Tes: alasan panjang dirender utuh dengan `whitespace-pre-line`; tanpa alasan → "—"; lampiran → tombol "Lampiran 1" membuka pratinjau dengan `src` dari `hrefFor`; `actions` dirender di `[data-slot="dialog-footer"]`; tanpa `actions` ada tombol "Tutup".
- [ ] Implementasi; tes pratinjau lembur yang ada tetap hijau. Commit.

### Task 3: ESS Persetujuan Tim + Perjalanan Dinas tim

**Files:** `features/hris/team-approvals/components/persetujuan-tim-page.tsx`, `overtime-approve-dialog.tsx`, `features/hris/business-trips/components/team-trip-approvals.tsx` (+ test yang ada)

- [ ] Tiap baris cuti/lembur/trip: tombol "Detail" (`aria-label="Detail {cuti|lembur|perjalanan dinas} {nama}"`), alasan di baris `line-clamp-2` (bukan `truncate`). Lampiran cuti di baris memakai `AttachmentPreviewButtons label="Lampiran"`.
- [ ] Dialog Detail: cuti (jenis, periode, jumlah hari, diajukan, alasan, lampiran); lembur (tanggal, jam, jenis hari, istirahat, jam dibayar, diajukan, alasan, dokumentasi); trip (periode, tujuan, keperluan). Kaki: Setujui/Tolak memanggil alur yang sama (lembur Setujui → `OvertimeApproveDialog`).
- [ ] `OvertimeApproveDialog` menampilkan alasan (prop `reason?: string | null`). Dialog Tolak menampilkan ringkasan (jenis + tanggal + alasan pengajuan) di atas kolom alasan penolakan.
- [ ] Tes: klik Detail cuti → alasan lengkap + tombol Lampiran; Setujui dari dialog memanggil mutate approve; Tolak dari dialog membuka dialog alasan. Commit.

### Task 4: ESS Kasbon tim

**Files:** `features/hris/ess/components/pinjaman-page.tsx`, `features/hris/ess/{api,mutations}.ts` (+ tipe `notes`), test baru/yang ada.

- [ ] `decideTeamLoan(id, approved, rejectionReason?)` mengirim `rejection_reason`.
- [ ] Tombol Detail (jenis, nominal, tenor, keperluan, catatan, diajukan) dengan Setujui/Tolak; Tolak membuka `ConfirmDialog` berkolom alasan wajib.
- [ ] Tes: tolak tanpa alasan → tidak memanggil API; dengan alasan → body memuat `rejection_reason`. Commit.

### Task 5: Halaman HR

**Files:** `features/hris/leaves/components/leaves-page.tsx`, `features/hris/overtime/components/overtime-table.tsx`, `features/hris/attendance-requests/components/attendance-requests-tab.tsx`, `features/hris/business-trips/components/business-trips-tab.tsx`, `features/hris/loans/components/loans-table.tsx` (+ tes yang ada)

- [ ] Cuti HR: tampilkan Lampiran (pratinjau) di kartu; tombol Detail; Setujui/Tolak di dialog memakai `ApprovalButtons` bila `canDecide` dan pending.
- [ ] Lembur HR: tombol Detail per baris (alasan, dokumentasi, penolakan, koreksi); aksi tetap di baris (dropdown jenis hari ada di baris).
- [ ] Pengajuan Absensi: Detail (diajukan vs tercatat, alasan, foto bukti pratinjau) + Setujui/Tolak memakai `request(r, …)`; foto di baris pakai pratinjau.
- [ ] Perjalanan Dinas HR: Detail (tujuan, keperluan, status/keputusan) + aksi yang sama.
- [ ] Pinjaman HR: Detail (keperluan lengkap, catatan, nominal, cicilan, tenor, status, alasan tolak) + Setujui/Tolak bila `isAwaitingHr`.
- [ ] Tes per halaman: Detail membuka dialog berisi alasan lengkap; cuti HR menampilkan Lampiran. Commit.

### Task 6: Gerbang + dokumen
- [ ] `go vet`, `go test` paket tersentuh; frontend `pnpm tsc --noEmit`, `pnpm lint`, `pnpm test` (seluruh suite).
- [ ] Probe Playwright di staging setelah deploy: dialog Detail tanpa elemen meluber (1280/390).
- [ ] Automation Log EPIC-072 + README registry. Commit.
