# Hak Akses HRIS per Akun Email — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tiga akun HR mendapat hak akses masing-masing, dan data Payroll, Recruitment, serta KPI benar-benar tertutup (menu, halaman, dan API) bagi akun yang tidak berhak:

| Jabatan | Email | Hak akses | Role IAM baru |
|---|---|---|---|
| Manager | paddyhumanresources@gmail.com | Seluruh akses HRIS | `hris_manager` |
| Staff | paddy.peopledevelopment@gmail.com | Seluruh akses kecuali Payroll | `hris_non_payroll` |
| Staff | paddyhroperation@gmail.com | Seluruh akses kecuali Recruitment dan KPI | `hris_non_rekrutmen_kpi` |

**Architecture:** Hak akses tetap memakai IAM yang sudah ada (`iam.roles`, `iam.role_menu_permissions`, `iam.user_roles`).
- **Role:** tiga role IAM baru diturunkan dari `admin_hris` lewat fungsi SQL `iam.sync_hris_staff_roles()`. Fungsi ini bisa dijalankan ulang, jadi grant baru `admin_hris` di masa depan cukup disalin dengan memanggilnya lagi.
- **Penugasan akun:** dilakukan skrip ops per email. Data akun live tidak dimasukkan ke migrasi.
- **Celah yang ditutup:** banyak route masih memakai *peran aplikasi* (`hrd` ⇒ "HR"), bukan menu. Akibatnya ketiga akun (semuanya `hrd`) lolos walau menunya dicabut. Route tersebut kini juga mensyaratkan menu yang relevan:
  - Payslip semua karyawan → `hris.compensation`
  - KPI/Review/Task Departemen/Logbook lintas departemen → `hris.performance`
  - Dashboard & Analytics rekrutmen → `hris.recruitment` atau `hris.insights.analytics`
- **Angka gaji di Kontrak & Riwayat Kepegawaian** tidak dikirim API kepada akun tanpa `hris.compensation`. PDF kontrak dan scan kontrak bertanda tangan (keduanya memuat upah) juga hanya untuk akun dengan akses itu.

**Tech Stack:** Go modular monolith (`backend/internal/modules/{payroll,hris,insights}`), PostgreSQL 16 (migrasi `backend/database/migrations/deltas`), Next.js (versi repo; baca `frontend/node_modules/next/dist/docs` sebelum menulis halaman), TanStack Query, vitest + testing-library.

**Spec:** tidak ada file spec terpisah. Keputusan user (percakapan 19-10-2026) adalah otoritasnya dan disalin utuh di bagian **Keputusan** di bawah.

## Keputusan (dari user, 19-10-2026)

1. **Satu peran per akun.** Tiga role IAM baru diturunkan dari `admin_hris`. Peran aplikasi tetap `hrd`.
2. **"Seluruh akses" = semua aksi.** Untuk setiap menu yang diberikan, aksinya adalah semua aksi menu itu (`iam.menus.permission_context.actions`) digabung dengan aksi `admin_hris` saat ini.
3. **Kecuali Payroll** = menu Penggajian (Run Penggajian, Salary, Biaya Gaji per Toko) **dan** data gaji di detail karyawan. **Pinjaman tetap boleh** (menu Kepegawaian → Pinjaman).
4. **Kecuali KPI** = seluruh menu Kinerja: KPI Scorecard, KPI Templates, Konfigurasi KPI, Task Departemen, Logbook, Logbook List, Performance Review.
5. **Kontrak untuk akun Non-Payroll:** "Sembunyikan angka gaji".
   - Kontrak tetap bisa dilihat, dibuat, dan diaktifkan.
   - Gaji pokok, tunjangan, dan uang kompensasi tidak tampil dan tidak dikirim API.
   - PDF kontrak (memuat upah) hanya untuk akun yang punya akses Payroll.
6. **AdminHRIS** (role `admin_hris`) tetap ada dan tetap mengatur Payroll, Recruitment, dan Absensi. Role ini tidak diubah.

## Global Constraints

- **Kode role (persis):** `hris_manager` "HR Manager", `hris_non_payroll` "HR Staff (Non-Payroll)", `hris_non_rekrutmen_kpi` "HR Staff (Non-Rekrutmen & KPI)".
- **Subtree yang dikecualikan:**
  - `hris_non_payroll`: `hris.compensation` beserta seluruh anaknya.
  - `hris_non_rekrutmen_kpi`: `hris.recruitment`, `hris.performance`, dan `hris.insights.analytics` (Analytics berisi data kandidat — lihat Ruling R1), beserta seluruh anaknya.
  - Menu grup (`hris.compensation`, `hris.recruitment`, `hris.performance`) ikut dicabut. Pencocokan prefix IAM menganggap kode grup itu sendiri sebagai grant.
- **Kunci data gaji:**
  - kontrak: `base_salary`, `allowances`, `compensation_amount`;
  - riwayat: `prev_salary`, `new_salary`.
- **Pesan error** (bahasa user):
  - PDF/scan kontrak: `Dokumen kontrak memuat gaji — hanya untuk akun dengan akses Penggajian`
  - Selain itu memakai pesan yang sudah ada (`Insufficient permissions`).
- **Fallback halaman terlarang:** `/dashboard/me`, bukan `/dashboard`. Ini mencegah redirect berputar untuk peran yang beranda-nya halaman yang dijaga.
- **Jangan ubah** perilaku peran lain. Data live per 19-10-2026: hanya 3 akun `hrd` (ketiganya `admin_hris`) dan 2 `super_admin` (role `super_admin` memegang semua 44 menu HRIS). Role `hrd`, `admin`, `direksi`, dan `hiring_manager` sudah memegang menu yang disyaratkan, jadi tidak berubah.
- **Test integrasi Go:**
  - Pakai `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'` (container `paddydev-sdd-testdb`). **Bukan** port 55432 (DB proyek lain).
  - Go ada di `$HOME/.local/go/bin`.
  - 27 kegagalan lama yang sudah diketahui sama persis dengan di main, jadi bukan regresi: hris TestAttendancePeriodClosing, spa TestTherapistPayrollTwiceAMonth, dan 403 di site/storedvalue/auth/gym/crm.
- **Fixture staff di test:** buat staff **sebelum** transaksi harness, atau daftarkan `t.Cleanup(rollback)` **setelah** `CreateStaff`. Kalau tidak, cleanup menghapus akun sambil transaksi masih memegang lock (deadlock).
- **Commit:** satu per task, dengan pesan diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. File berisi email/kredensial (CSV akun, dsb.) **tidak** di-commit. Skrip ops yang memuat 3 email kantor boleh di-commit; email itu sudah ada di seeder yang ter-commit.
- **Live:** tidak ada migrasi, skrip ops, atau deploy ke live sebelum user menyetujui hasil di stack uji salinan DB (Task 8). Backup DB dulu sebelum perubahan live.

## Ruling (keputusan turunan, tolong dikoreksi saat review)

- **R1 — Analytics = Recruitment.**
  - Menu *Laporan → Analytics* (`/dashboard/hris/analytics`) dan Dashboard Rekrutmen membaca `recruitment.candidates`. Untuk akun Non-Rekrutmen keduanya ikut ditutup.
  - *Laporan HRIS* (`hris.insights.reports`) tetap boleh.
- **R2 — Dokumen rekrutmen di profil karyawan tetap tampil** untuk semua akun yang memegang Kepegawaian (`/employees/{id}/recruitment-documents`). Isinya CV/berkas lamaran karyawan yang sudah jadi data kepegawaian, dan HR Operation memerlukannya. Kalau ingin ditutup juga, cukup ganti guard route itu ke `iam.HrisRecruitment`.
- **R3 — Scan kontrak bertanda tangan:**
  - Akun Non-Payroll tetap bisa **mengunggah/menghapus** scan (proses aktivasi kontrak tidak macet).
  - **Melihat** scan hanya untuk akun dengan akses Penggajian, karena scan sama dengan PDF kontrak (memuat upah).
- **R4 — Gaji kontrak yang dibuat akun Non-Payroll:**
  - Nilai `base_salary` dari klien diabaikan. Kontrak memakai gaji aktif karyawan (perilaku default yang sudah ada).
  - Aksi `edit` tidak mengubah gaji.
  - Dengan begitu aktivasi tidak menulis angka karangan ke struktur gaji.
- **R5 — Riwayat kepegawaian:**
  - `prev_salary`/`new_salary` dihapus dari **GET** untuk akun tanpa Penggajian.
  - Balasan POST tidak diubah (tidak ada UI yang menulis gaji di riwayat). Test lama mengandalkan balasan POST itu.
- **R6 — Asisten AI** (`insights/summary.go`) hanya memuat *jumlah* baris lintas modul, tanpa angka gaji atau data per orang. Tidak diubah di rencana ini; dicatat sebagai minor.
- **R7 — Halaman Kinerja yang sudah `requireUser()`** (performance, dept-tasks, kpi-config) dan halaman logbook tidak diberi guard halaman. Karyawan biasa memakainya.
  - Akun Non-KPI tidak melihatnya di sidebar (menu dicabut).
  - API-nya kini memperlakukan akun itu sebagai karyawan biasa (data sendiri saja), lewat Task 2 & 4.

## Review Focus

1. **Akun `hrd` tanpa `hris.compensation` membuka slip orang lain** lewat `GET /api/hris/payslips?employee_id=<lain>` atau `/payslips/{id}/pdf`. Harapannya: hanya slip sendiri; PDF orang lain 403. Test: `TestHRWithoutPayrollMenuSeesOnlyOwnPayslips` (Task 2).
2. **Akun Non-Payroll mengirim `base_salary` lewat klien** (create/edit kontrak). Harapannya: diabaikan, gaji kontrak = gaji aktif. Test: `TestContractSalaryHiddenWithoutPayrollMenu` (Task 3).
3. **Pinjaman untuk akun Non-Payroll:** halaman `/dashboard/hris/loans` dan approve pinjaman harus tetap jalan dengan menu `hris.kepegawaian.loans` saja. Test: `TestLoanApprovalFollowsLoansMenu` (Task 2) + cek halaman di Task 8.
4. **Beranda akun Non-Rekrutmen:** `/dashboard` mengarahkan `hrd` ke Dashboard Rekrutmen, yang kini 403 bagi akun ini. Harapannya: diarahkan ke `/dashboard/employees`, tanpa redirect berputar. Test: `homeFor` unit test (Task 6).
5. **Peran lain tidak berubah:** `direksi` (Analytics saja) dan `hiring_manager` (`hris.recruitment` + analytics) tetap bisa membuka Dashboard Rekrutmen & Analytics. Test: `TestRecruitmentReadersKeepAnalyticsHolders` (Task 5).

---

### Task 1: Role IAM per akun + prefix menu baru

**Files:**
- Create: `backend/database/migrations/deltas/20261019100000_iam_hris_staff_roles.sql`
- Modify: `frontend/src/lib/iam/prefixes.ts` (setelah `hrisRecruitment`, ±baris 34)
- Regenerate: `backend/internal/platform/iam/prefixes_gen.go` (`make -C backend generate`)
- Test: `backend/internal/platform/auth/hris_staff_roles_integration_test.go`

**Interfaces:**
- Produces:
  - SQL function `iam.sync_hris_staff_roles() RETURNS void` (idempoten);
  - roles `hris_manager`, `hris_non_payroll`, `hris_non_rekrutmen_kpi`;
  - Go prefixes `iam.HrisLoans = []string{"hris.kepegawaian.loans", "hris.compensation"}` dan `iam.HrisAnalytics = []string{"hris.insights.analytics"}`;
  - TS `IAM.hrisLoans`, `IAM.hrisAnalytics`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/platform/auth/hris_staff_roles_integration_test.go`:

```go
package auth_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/testutil"
)

func roleGrants(t *testing.T, tx pgx.Tx, role string) map[string][]string {
	t.Helper()
	rows, err := tx.Query(context.Background(), `SELECT m.code, p.granted_actions
		FROM iam.role_menu_permissions p
		JOIN iam.roles r ON r.id = p.role_id
		JOIN iam.menus m ON m.id = p.menu_id
		WHERE r.code = $1 AND p.is_active AND m.deleted_at IS NULL`, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var code string
		var raw []byte
		if err := rows.Scan(&code, &raw); err != nil {
			t.Fatal(err)
		}
		var acts []string
		_ = json.Unmarshal(raw, &acts)
		slices.Sort(acts)
		out[code] = acts
	}
	return out
}

func under(code string, prefixes ...string) bool {
	for _, p := range prefixes {
		if code == p || strings.HasPrefix(code, p+".") {
			return true
		}
	}
	return false
}

func TestHrisStaffRolesDeriveFromAdminHris(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	src := roleGrants(t, tx, "admin_hris")
	if len(src) == 0 {
		t.Skip("admin_hris has no grants in this database")
	}
	var menuActs = map[string][]string{}
	rows, _ := tx.Query(ctx, `SELECT code, coalesce(permission_context->'actions', '[]'::jsonb) FROM iam.menus WHERE deleted_at IS NULL`)
	for rows.Next() {
		var code string
		var raw []byte
		_ = rows.Scan(&code, &raw)
		var a []string
		_ = json.Unmarshal(raw, &a)
		menuActs[code] = a
	}
	rows.Close()

	want := func(excluded ...string) map[string][]string {
		out := map[string][]string{}
		for code, acts := range src {
			if under(code, excluded...) {
				continue
			}
			all := slices.Concat(acts, menuActs[code])
			slices.Sort(all)
			out[code] = slices.Compact(all)
		}
		return out
	}
	for role, excluded := range map[string][]string{
		"hris_manager":           nil,
		"hris_non_payroll":       {"hris.compensation"},
		"hris_non_rekrutmen_kpi": {"hris.recruitment", "hris.performance", "hris.insights.analytics"},
	} {
		got, exp := roleGrants(t, tx, role), want(excluded...)
		for code, acts := range exp {
			if !slices.Equal(got[code], acts) {
				t.Errorf("%s %s = %v, want %v", role, code, got[code], acts)
			}
		}
		for code := range got {
			if _, ok := exp[code]; !ok {
				t.Errorf("%s has extra grant %s", role, code)
			}
		}
	}
	// Pinjaman stays for Non-Payroll; Laporan HRIS stays for Non-Rekrutmen & KPI.
	if np := roleGrants(t, tx, "hris_non_payroll"); !slices.Contains(np["hris.kepegawaian.loans"], "approve") {
		t.Errorf("non-payroll loans = %v", np["hris.kepegawaian.loans"])
	}
	if nr := roleGrants(t, tx, "hris_non_rekrutmen_kpi"); nr["hris.insights.reports"] == nil {
		t.Error("non-rekrutmen lost Laporan HRIS")
	}

	// Re-running deactivates a grant the source no longer has.
	if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions p SET is_active = false
		FROM iam.roles r, iam.menus m WHERE r.id = p.role_id AND m.id = p.menu_id
		AND r.code = 'admin_hris' AND m.code = 'hris.kepegawaian.holidays'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	if roleGrants(t, tx, "hris_manager")["hris.kepegawaian.holidays"] != nil {
		t.Error("sync kept a grant admin_hris dropped")
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/platform/auth -run TestHrisStaffRolesDeriveFromAdminHris -v`
Expected: FAIL — `function iam.sync_hris_staff_roles() does not exist`.

- [ ] **Step 3: Tulis migrasi**

`backend/database/migrations/deltas/20261019100000_iam_hris_staff_roles.sql`:

```sql
-- =============================================================================
-- Hak akses HRIS per akun (keputusan user 19-10-2026). Tiga role turunan
-- admin_hris dengan semua aksi menu:
--   hris_manager            seluruh HRIS
--   hris_non_payroll        tanpa Penggajian (hris.compensation)
--   hris_non_rekrutmen_kpi  tanpa Rekrutmen, Kinerja, dan Analytics rekrutmen
-- admin_hris sendiri tidak diubah. Panggil ulang iam.sync_hris_staff_roles()
-- setiap kali grant admin_hris berubah. Penugasan akun: skrip ops
-- database/sql/fixes/2026-10-21_hris_akses_per_akun.sql.
-- =============================================================================
INSERT INTO iam.roles (code, name, description, is_system, is_active) VALUES
  ('hris_manager', 'HR Manager', 'Seluruh akses HRIS + Area Karyawan', false, true),
  ('hris_non_payroll', 'HR Staff (Non-Payroll)', 'Seluruh akses HRIS kecuali Penggajian', false, true),
  ('hris_non_rekrutmen_kpi', 'HR Staff (Non-Rekrutmen & KPI)', 'Seluruh akses HRIS kecuali Rekrutmen dan Kinerja', false, true)
ON CONFLICT (code) DO NOTHING;

CREATE OR REPLACE FUNCTION iam.sync_hris_staff_roles() RETURNS void
LANGUAGE sql AS $$
  WITH spec(code, excluded) AS (VALUES
    ('hris_manager', ARRAY[]::text[]),
    ('hris_non_payroll', ARRAY['hris.compensation']),
    ('hris_non_rekrutmen_kpi', ARRAY['hris.recruitment', 'hris.performance', 'hris.insights.analytics'])),
  src AS (
    SELECT m.id AS menu_id, m.code,
      (SELECT jsonb_agg(DISTINCT a ORDER BY a)
         FROM jsonb_array_elements_text(rmp.granted_actions || coalesce(m.permission_context->'actions', '[]'::jsonb)) a) AS actions
    FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active),
  want AS (
    SELECT r.id AS role_id, s.menu_id, s.actions
    FROM spec
    JOIN iam.roles r ON r.code = spec.code
    JOIN src s ON NOT EXISTS (
      SELECT 1 FROM unnest(spec.excluded) p WHERE s.code = p OR s.code LIKE p || '.%')),
  upserted AS (
    INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
    SELECT role_id, menu_id, actions, true FROM want
    ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
    DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true, updated_at = now()
    RETURNING 1)
  UPDATE iam.role_menu_permissions p SET is_active = false, updated_at = now()
  FROM iam.roles r
  WHERE r.id = p.role_id AND r.code IN (SELECT code FROM spec) AND p.is_active
    AND NOT EXISTS (SELECT 1 FROM want w WHERE w.role_id = p.role_id AND w.menu_id = p.menu_id);
$$;

SELECT iam.sync_hris_staff_roles();
```

Lalu terapkan ke DB test:

```bash
cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply
```

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: perintah Step 2.
Expected: PASS.

- [ ] **Step 5: Tambah prefix menu dan regenerate**

Di `frontend/src/lib/iam/prefixes.ts`, tepat setelah baris `hrisRecruitment: ["hris.recruitment"],`:

```ts
  /** Pinjaman (Kepegawaian → Pinjaman); Penggajian tetap boleh seperti dulu. */
  hrisLoans: ["hris.kepegawaian.loans", "hris.compensation"],
  /** Analytics rekrutmen — membaca data kandidat. */
  hrisAnalytics: ["hris.insights.analytics"],
```

Run: `make -C backend generate && cd frontend && npx vitest run src/lib/iam`
Expected:
- `prefixes_gen.go` memuat `HrisLoans` dan `HrisAnalytics`;
- `go-routes.generated.json` tidak berubah (atau hanya urutan);
- test iam PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/database/migrations/deltas/20261019100000_iam_hris_staff_roles.sql backend/internal/platform/auth/hris_staff_roles_integration_test.go frontend/src/lib/iam/prefixes.ts backend/internal/platform/iam/prefixes_gen.go
git commit -m "feat(iam): role HR per akun turunan admin_hris (manager, non-payroll, non-rekrutmen-kpi)"
```

---

### Task 2: Payroll — slip, pinjaman, dan Kinerja mengikuti menu

**Files:**
- Modify: `backend/internal/modules/payroll/actor.go`
- Modify: `backend/internal/modules/payroll/domain/access.go:38-52`
- Modify: `backend/internal/modules/payroll/payslips.go:28`
- Modify: `backend/internal/modules/payroll/payslip_pdf.go:60`
- Modify: `backend/internal/modules/payroll/loans.go:257`
- Modify (ganti nama field `IsHR` → `PerformanceHR`): `payroll/kpi.go:254`, `payroll/performance.go` (semua `a.IsHR`), `payroll/depttasks.go` (semua `me.IsHR`/`a.IsHR`)
- Test: `backend/internal/modules/payroll/access_split_integration_test.go`, `backend/internal/modules/payroll/domain/access_test.go` (tambah kasus; buat file bila belum ada)

**Interfaces:**
- Consumes: `iam.HrisLoans` (Task 1).
- Produces:
  - `actor.PerformanceHR bool`, `actor.Payroll bool` (field `IsHR` dihapus);
  - `domain.ResolvePayslipScope(role string, payroll bool, actorEmployeeID *string, param *string) PayslipScope`.

- [ ] **Step 1: Tulis test yang gagal**

`payroll/domain/access_test.go` (tambahkan):

```go
func TestResolvePayslipScopeNeedsPayrollMenu(t *testing.T) {
	me := "emp-1"
	other := "emp-2"
	if got := ResolvePayslipScope("hrd", false, &me, &other); !got.MeView || got.EmployeeID != me {
		t.Fatalf("hrd without Penggajian = %+v, want own slips", got)
	}
	if got := ResolvePayslipScope("hrd", true, &me, &other); got.MeView || got.EmployeeID != other {
		t.Fatalf("hrd with Penggajian = %+v, want the requested employee", got)
	}
}
```

`payroll/access_split_integration_test.go`:

```go
package payroll

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// hrStaff is an hrd account holding only the given menus. The rollback is
// registered after CreateStaff so the fixture goes before the account.
func hrStaff(e *env, menus map[string][]string) testutil.Staff {
	e.t.Helper()
	s := testutil.CreateStaff(e.t, testutil.StaffOptions{Role: "hrd", Menus: menus})
	e.t.Cleanup(func() { _ = e.tx.Rollback(context.Background()) })
	return s
}

func TestHRWithoutPayrollMenuSeesOnlyOwnPayslips(t *testing.T) {
	e := setup(t)
	nonPay := hrStaff(e, map[string][]string{"hris.kepegawaian": nil})
	worker := e.employee("Slip Uji", "", true)
	e.salary(worker, e.co, 5000000)
	run := e.newRun(6, 2099, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+run+"/calculate", nil, 200)

	slips := e.call(&e.hr, "GET", "/api/hris/payslips?payroll_run_id="+run, nil, 200)["data"].([]any)
	if len(slips) == 0 {
		t.Fatal("HR with Penggajian should list the run's slips")
	}
	id := slips[0].(map[string]any)["id"].(string)
	if got := e.call(&nonPay, "GET", "/api/hris/payslips?employee_id="+worker, nil, 200)["data"].([]any); len(got) != 0 {
		t.Fatalf("HR without Penggajian sees %d slips, want 0", len(got))
	}
	if rec := e.raw(&nonPay, "GET", "/api/hris/payslips/"+id+"/pdf", nil); rec.Code != 403 {
		t.Fatalf("pdf = %d, want 403", rec.Code)
	}
}

func TestLoanApprovalFollowsLoansMenu(t *testing.T) {
	e := setup(t)
	loansHR := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	e.employee("HR Pinjaman", loansHR.UserID, true)
	worker := e.employee("Peminjam", e.staff.UserID, true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, effective_date) VALUES ($1, $2, 5000000, '2026-01-01')`, worker, e.co)
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{
		"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	e.call(&loansHR, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 200)
}

func TestHRWithoutKinerjaMenuIsNotPerformanceHR(t *testing.T) {
	e := setup(t)
	opsHR := hrStaff(e, map[string][]string{"hris.kepegawaian": nil})
	if got := e.call(&opsHR, "GET", "/api/hris/performance/cycles", nil, 200)["data"].(map[string]any); got["is_hr"] != false {
		t.Fatalf("HR without Kinerja is_hr = %v, want false", got["is_hr"])
	}
	if got := e.call(&e.hr, "GET", "/api/hris/performance/cycles", nil, 200)["data"].(map[string]any); got["is_hr"] != true {
		t.Fatalf("HR with Kinerja is_hr = %v, want true", got["is_hr"])
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/payroll/... -run 'TestResolvePayslipScopeNeedsPayrollMenu|TestHRWithoutPayrollMenuSeesOnlyOwnPayslips|TestLoanApprovalFollowsLoansMenu|TestHRWithoutKinerjaMenuIsNotPerformanceHR' -v`
Expected:
- compile error `too many arguments in call to ResolvePayslipScope` (domain);
- setelah signature sementara disesuaikan: slips = 1 (bukan 0), pdf 200, approve 403, `is_hr` true. Semuanya FAIL.

- [ ] **Step 3: Implementasi**

`payroll/domain/access.go` — ganti `ResolvePayslipScope`:

```go
// ResolvePayslipScope: employee_id=me is personal for every role; a full
// view needs a PayslipFullRoles role AND the Penggajian menu (payroll), so
// an HR account without Penggajian is forced to its own slips.
func ResolvePayslipScope(role string, payroll bool, actorEmployeeID *string, param *string) PayslipScope {
	full := payroll && slices.Contains(PayslipFullRoles, role)
	// ... sisa fungsi tidak berubah
```

`payroll/actor.go`:

```go
type actor struct {
	UserID     string
	Role       string
	EmployeeID *string
	// PerformanceHR: an HR role holding the Kinerja menu (hris.performance)
	// sees and manages every employee's KPI, reviews and department tasks.
	PerformanceHR bool
	// Payroll: the account holds the Penggajian menu (hris.compensation).
	Payroll bool
}
```

dan di akhir `requireActor`, ganti `a.IsHR = domain.IsHR(a.Role)` dengan:

```go
	granted, err := h.auth.GrantedMenuCodes(r.Context(), su.ID, a.Role)
	if err != nil {
		return nil, err
	}
	a.PerformanceHR = domain.IsHR(a.Role) && iam.HasAnyMenuPrefix(granted, iam.HrisPerformance)
	a.Payroll = iam.HasAnyMenuPrefix(granted, iam.HrisCompensation)
	return a, nil
```

(tambahkan import `"nuhabit/backend/internal/platform/iam"`).

`payroll/payslips.go:28`: `scope := domain.ResolvePayslipScope(a.Role, a.Payroll, a.EmployeeID, employeeParam)`

`payroll/payslip_pdf.go:60`:

```go
	if !(domain.IsHR(a.Role) && a.Payroll) && (a.EmployeeID == nil || *a.EmployeeID != slip.employee.ID) {
```

`payroll/loans.go:257`: `u, err := h.auth.RequireMenuPrefix(r, iam.HrisLoans...)`

`kpi.go`, `performance.go`, `depttasks.go`: ganti setiap `a.IsHR` / `me.IsHR` menjadi `a.PerformanceHR` / `me.PerformanceHR`. Ini mekanis; compiler menunjukkan sisanya. Kunci JSON `"is_hr"` tetap sama.

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: perintah Step 2, lalu seluruh paket: `TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/payroll/... 2>&1 | tail -30`
Expected: 4 test baru PASS; tidak ada kegagalan baru di luar daftar baseline. Test lama memakai `e.hr` yang memegang `hris.compensation` + `hris.performance`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/payroll
git commit -m "fix(payroll): slip semua karyawan & Kinerja HR mengikuti menu, approve pinjaman lewat menu Pinjaman"
```

---

### Task 3: HRIS — angka gaji kontrak & riwayat hanya untuk Penggajian

**Files:**
- Modify: `backend/internal/modules/hris/row.go` (tambah `Del`)
- Create: `backend/internal/modules/hris/salary_visibility.go`
- Modify: `backend/internal/modules/hris/http_contracts.go` (listEmployeeContracts, createEmployeeContract, listContracts, patchContract)
- Modify: `backend/internal/modules/hris/http_files.go` (contractDocument, signedDocument)
- Modify: `backend/internal/modules/hris/http_employees.go:290-303` (listEmploymentHistory)
- Test: `backend/internal/modules/hris/salary_visibility_integration_test.go`

**Interfaces:**
- Produces:
  - `func (r *Row) Del(key string)`;
  - `func (h handlers) canSeeSalary(r *http.Request, u *auth.User) (bool, error)`;
  - `func hideKeys(rows []*Row, keys []string)`;
  - vars `contractSalaryKeys`, `historySalaryKeys`;
  - respons `GET /api/hris/employees/{id}/contracts` = `{ "data": [...], "can_view_salary": bool }` (dipakai Task 6).

- [ ] **Step 1: Tulis test yang gagal**

```go
package hris

import (
	"net/http"
	"testing"
)

func TestContractSalaryHiddenWithoutPayrollMenu(t *testing.T) {
	h := newHarness(t)
	ops, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	pay, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{status: "contract"})
	base := "/api/hris/employees/" + e + "/contracts"
	salaryKeys := []string{"base_salary", "allowances", "compensation_amount"}
	noSalary := func(where string, row map[string]any) {
		t.Helper()
		for _, k := range salaryKeys {
			if _, leaked := row[k]; leaked {
				t.Fatalf("%s leaks %s: %v", where, k, row)
			}
		}
	}

	// Non-Payroll may create; the client's salary is ignored (active salary used).
	created := h.do(&ops, http.MethodPost, base, map[string]any{
		"contract_type": "pkwt", "start_date": "2026-01-01", "end_date": "2027-01-01", "base_salary": 1})
	expect(t, created, 201, "")
	noSalary("create", created.data())
	id := created.data()["id"].(string)

	list := h.do(&ops, http.MethodGet, base, nil)
	expect(t, list, 200, "")
	noSalary("employee contracts", list.list()[0].(map[string]any))
	if list.body["can_view_salary"] != false {
		t.Fatalf("can_view_salary = %v", list.body["can_view_salary"])
	}
	all := h.do(&ops, http.MethodGet, "/api/hris/contracts", nil)
	expect(t, all, 200, "")
	for _, r := range all.list() {
		noSalary("contracts list", r.(map[string]any))
	}

	// Editing the draft with a salary leaves it unchanged.
	expect(t, h.do(&ops, http.MethodPatch, "/api/hris/contracts/"+id,
		map[string]any{"action": "edit", "start_date": "2026-01-01", "base_salary": 2}), 200, "")

	full := h.do(&pay, http.MethodGet, base, nil)
	if got := full.list()[0].(map[string]any)["base_salary"]; got != "4800000.00" || full.body["can_view_salary"] != true {
		t.Fatalf("payroll view base_salary = %v, can_view_salary = %v", got, full.body["can_view_salary"])
	}

	const docMsg = "Dokumen kontrak memuat gaji — hanya untuk akun dengan akses Penggajian"
	expect(t, h.do(&ops, http.MethodGet, "/api/hris/contracts/"+id+"/document", nil), 403, docMsg)
	expect(t, h.do(&ops, http.MethodGet, "/api/hris/contracts/"+id+"/signed-document", nil), 403, docMsg)
	if got := h.do(&pay, http.MethodGet, "/api/hris/contracts/"+id+"/document", nil); got.status != 200 {
		t.Fatalf("payroll pdf = %d %s", got.status, got.raw)
	}
}

func TestEmploymentHistorySalaryHiddenWithoutPayrollMenu(t *testing.T) {
	h := newHarness(t)
	ops, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	pay, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{})
	expect(t, h.do(&ops, http.MethodPost, "/api/hris/employment-history", map[string]any{
		"employee_id": e, "change_type": "promotion", "effective_date": "2026-09-01", "new_salary": "6500000", "prev_salary": "5000000",
	}), 201, "")
	url := "/api/hris/employment-history?employee_id=" + e
	row := h.do(&ops, http.MethodGet, url, nil).list()[0].(map[string]any)
	for _, k := range []string{"prev_salary", "new_salary"} {
		if _, leaked := row[k]; leaked {
			t.Fatalf("history leaks %s: %v", k, row)
		}
	}
	if got := h.do(&pay, http.MethodGet, url, nil).list()[0].(map[string]any)["new_salary"]; got != "6500000.00" {
		t.Fatalf("payroll history new_salary = %v", got)
	}
}
```

Catatan untuk executor:
- `4800000.00` = gaji aktif fixture `h.employee` (lihat `TestEmployeeContracts`).
- Bila `signed-document` mengembalikan 404 lebih dulu karena kontrak belum punya scan, guard harus dicek **sebelum** pencarian dokumen, supaya 403 tetap terlihat.

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestContractSalaryHiddenWithoutPayrollMenu|TestEmploymentHistorySalaryHiddenWithoutPayrollMenu' -v`
Expected: FAIL — `create leaks base_salary`.

- [ ] **Step 3: Implementasi**

`hris/row.go`, setelah `Get`:

```go
// Del removes a key (a no-op when absent), keeping the other keys' order.
func (r *Row) Del(key string) {
	if _, ok := r.vals[key]; !ok {
		return
	}
	delete(r.vals, key)
	r.keys = slices.DeleteFunc(r.keys, func(k string) bool { return k == key })
}
```

(tambah import `"slices"` bila belum ada).

`hris/salary_visibility.go`:

```go
package hris

import (
	"net/http"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
)

// Salary figures on contracts and employment history belong to Penggajian
// (hris.compensation): Kepegawaian accounts without it manage contracts but
// never see or set the amounts (keputusan user 19-10-2026).
var (
	contractSalaryKeys = []string{"base_salary", "allowances", "compensation_amount"}
	historySalaryKeys  = []string{"prev_salary", "new_salary"}
)

const contractDocumentPayrollOnly = "Dokumen kontrak memuat gaji — hanya untuk akun dengan akses Penggajian"

// canSeeSalary: an account without a profile row (u nil) never sees salaries.
func (h handlers) canSeeSalary(r *http.Request, u *auth.User) (bool, error) {
	if u == nil {
		return false, nil
	}
	return h.auth.HasMenuPrefix(r.Context(), u, iam.HrisCompensation...)
}

// requireSalaryAccess guards the contract PDF and signed scan, which print the wage.
func (h handlers) requireSalaryAccess(r *http.Request, u *auth.User) error {
	ok, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Forbidden(contractDocumentPayrollOnly)
	}
	return nil
}

func hideKeys(rows []*Row, keys []string) {
	for _, row := range rows {
		if row == nil {
			continue
		}
		for _, k := range keys {
			row.Del(k)
		}
	}
}
```

`hris/http_contracts.go`:

- `listEmployeeContracts` — ganti `_ *auth.User` menjadi `u *auth.User`, lalu:

```go
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		hideKeys(rows, contractSalaryKeys)
	}
	return ok(w, obj("data", rows, "can_view_salary", see))
```

- `createEmployeeContract` — sesudah `formErr` dan sebelum `CreateEmployeeContract`:

```go
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		in.BaseSalary = nil // R4: the active salary is used
	}
```

  dan sebelum `reply(...)`: `if !see { hideKeys([]*Row{row}, contractSalaryKeys) }`. Sesuaikan tipe bila `row` bukan `*Row`; lihat tanda tangan `CreateEmployeeContract`.

- `listContracts` — ganti `_` menjadi `u`; sebelum `ok(...)`, `see` seperti di atas; `if !see { hideKeys(rows, contractSalaryKeys) }`.

- `patchContract` — sesudah `formErr`:

```go
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		in.BaseSalary, in.BaseSalarySent = nil, false // R4: edit keeps the salary
	}
```

  dan ganti `return reply(w, status, result.body())` dengan:

```go
	body := result.body()
	if !see {
		body.Del("compensation_amount")
	}
	return reply(w, status, body)
```

`hris/http_files.go`:
- `contractDocument`: tambahkan `if err := h.requireSalaryAccess(r, u); err != nil { return err }` tepat setelah `requireUUID`.
- `signedDocument`: ganti `_ *auth.User` menjadi `u *auth.User`, dan tambahkan guard yang sama sebagai baris pertama.

`hris/http_employees.go` `listEmploymentHistory` — sebelum `return ok(w, dataOf(rows))`:

```go
	u, err := h.auth.CurrentUser(r)
	if err != nil {
		return err
	}
	if see, err := h.canSeeSalary(r, u); err != nil {
		return err
	} else if !see {
		hideKeys(rows, historySalaryKeys)
	}
```

(`requireEmployeeAccess` sudah menjamin ada sesi.)

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: perintah Step 2, lalu `TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/hris 2>&1 | tail -30`
Expected:
- 2 test baru PASS;
- `TestEmployeeContracts` tetap PASS. Ia memakai `hr` tanpa Penggajian dan memeriksa `base_salary` di balasan create. **Ruling:** ubah staff test itu menjadi `[]string{menuKepegawaian, "hris.compensation"}`, karena test itu memang menguji angka gaji default;
- tidak ada kegagalan baru di luar baseline.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris
git commit -m "fix(hris): angka gaji & PDF kontrak hanya untuk akses Penggajian"
```

---

### Task 4: HRIS — Logbook lintas departemen butuh menu Kinerja

**Files:**
- Modify: `backend/internal/modules/hris/domain/insights.go:17-23`
- Modify: `backend/internal/modules/hris/service.go:55-62` (field Actor)
- Modify: `backend/internal/modules/hris/http_people.go:282-296` (withLogbook)
- Modify: `backend/internal/modules/hris/logbook.go:49,74-75,139,303`
- Test: `backend/internal/modules/hris/domain/insights_test.go` (tambah; buat bila belum ada)

**Interfaces:**
- Produces:
  - `domain.HasFullLogbookAccess(role string, performance bool) bool`;
  - `domain.CanReviewLogbook(role string, performance bool) bool`;
  - `Actor.PerformanceMenu bool` (diisi hanya oleh `withLogbook`).

- [ ] **Step 1: Tulis test yang gagal**

```go
func TestLogbookFullAccessNeedsKinerjaMenu(t *testing.T) {
	if HasFullLogbookAccess("hrd", false) || CanReviewLogbook("hrd", false) {
		t.Fatal("hrd without Kinerja must be limited to its own department")
	}
	if !HasFullLogbookAccess("hrd", true) || !CanReviewLogbook("hrd", true) {
		t.Fatal("hrd with Kinerja keeps full access")
	}
	if HasFullLogbookAccess("employee", true) {
		t.Fatal("non-HR roles never get full access")
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -run TestLogbookFullAccessNeedsKinerjaMenu -v`
Expected: FAIL — `too many arguments in call to HasFullLogbookAccess`.

- [ ] **Step 3: Implementasi**

`domain/insights.go`:

```go
// HasFullLogbookAccess: super_admin, admin and hrd holding the Kinerja menu
// (performance) see every department.
func HasFullLogbookAccess(role string, performance bool) bool {
	return performance && (role == "super_admin" || role == "admin" || role == "hrd")
}

// CanReviewLogbook: super_admin and hrd holding the Kinerja menu review entries.
func CanReviewLogbook(role string, performance bool) bool {
	return performance && (role == "super_admin" || role == "hrd")
}
```

`service.go` Actor: tambahkan field

```go
	// PerformanceMenu: the account holds the Kinerja menu (set by withLogbook).
	PerformanceMenu bool
```

`http_people.go` withLogbook, setelah `a, err := h.svc.Actor(...)` + cek error:

```go
		if a.PerformanceMenu, err = h.auth.HasMenuPrefix(r.Context(), u, iam.HrisPerformance...); err != nil {
			return err
		}
```

`logbook.go`: setiap `domain.HasFullLogbookAccess(a.Role)` → `domain.HasFullLogbookAccess(a.Role, a.PerformanceMenu)`; setiap `domain.CanReviewLogbook(a.Role)` → `domain.CanReviewLogbook(a.Role, a.PerformanceMenu)`.

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: `$HOME/.local/go/bin/go test ./internal/modules/hris/domain -v -run Logbook && TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/hris -run Logbook -v`
Expected: PASS.
- Bila test logbook integrasi lama gagal karena staff HR-nya tidak memegang `hris.performance`, **Ruling:** tambahkan `"hris.performance"` ke menu staff test itu. Test itu menguji akses penuh HR.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris
git commit -m "fix(hris): logbook lintas departemen hanya untuk HR dengan menu Kinerja"
```

---

### Task 5: Insights — Dashboard & Analytics rekrutmen

**Files:**
- Modify: `backend/internal/modules/insights/module.go:73`
- Modify: `backend/internal/modules/insights/dashboard.go:143,184,235`
- Modify: `backend/internal/modules/insights/dashboard_integration_test.go:68` (staff test memakai menu Analytics)
- Test: `backend/internal/modules/insights/recruitment_access_integration_test.go`

**Interfaces:**
- Consumes: `iam.HrisAnalytics` (Task 1).

- [ ] **Step 1: Tulis test yang gagal**

```go
package insights

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func TestRecruitmentReadersKeepAnalyticsHolders(t *testing.T) {
	reportsOnly := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.insights": nil, "hris.insights.reports": nil}})
	analytics := testutil.CreateStaff(t, testutil.StaffOptions{Role: "direksi", Menus: map[string][]string{
		"hris.insights.analytics": nil}})
	recruiter := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hiring_manager", Menus: map[string][]string{
		"hris.recruitment": nil}})
	h := newTxHarness(t)
	for _, path := range []string{"/api/dashboard/stats", "/api/dashboard/funnel", "/api/analytics/overview", "/api/analytics/brands"} {
		if c := h.get(path, reportsOnly); c.status != http.StatusForbidden {
			t.Errorf("%s reports-only = %d, want 403", path, c.status)
		}
		if c := h.get(path, analytics); c.status != http.StatusOK {
			t.Errorf("%s analytics = %d, want 200 (%s)", path, c.status, c.raw)
		}
	}
	if c := h.get("/api/dashboard/stats", recruiter); c.status != http.StatusOK {
		t.Errorf("dashboard recruiter = %d", c.status)
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/insights -run TestRecruitmentReadersKeepAnalyticsHolders -v`
Expected: FAIL — `reports-only = 200, want 403`.

- [ ] **Step 3: Implementasi**

`insights/module.go:73`:

```go
// recruitmentReaders open the recruitment dashboard: Rekrutmen, or the
// recruitment Analytics menu (direksi, hiring_manager). Laporan HRIS alone
// does not (keputusan 19-10-2026: akun Non-Rekrutmen).
var recruitmentReaders = append(append([]string{}, iam.HrisRecruitment...), iam.HrisAnalytics...)
```

`insights/dashboard.go` baris 143, 184, 235: `RequireMenuPrefix(r, iam.HrisInsights...)` → `RequireMenuPrefix(r, iam.HrisAnalytics...)`.

`dashboard_integration_test.go:68`: `"hris.insights": nil` → `"hris.insights.analytics": nil`. Analytics kini mensyaratkan menu Analytics; grup Laporan saja tidak cukup.

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: `TEST_DATABASE_URL=... $HOME/.local/go/bin/go test ./internal/modules/insights 2>&1 | tail -20`
Expected: PASS semua; tidak ada kegagalan baru.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/insights
git commit -m "fix(insights): dashboard & analytics rekrutmen butuh menu Rekrutmen atau Analytics"
```

---

### Task 6: Frontend — kontrak tanpa angka gaji, guard halaman, beranda HR

**Files:**
- Modify: `frontend/src/features/users/api.ts:155-165,233-234`
- Modify: `frontend/src/features/users/queries.ts:50-57`
- Modify: `frontend/src/features/users/components/contracts/contract-card.tsx`
- Modify: `frontend/src/features/users/components/contracts/contract-form-dialog.tsx:33-41,130-137`
- Modify: `frontend/src/features/users/components/contracts/contract-admin-dialog.tsx:30-38,127-135`
- Modify: `frontend/src/features/users/components/employee-contracts-tab.tsx`
- Modify: `frontend/src/features/users/components/employee-documents-tab.tsx`
- Modify (guard halaman, fallback `/dashboard/me`):
  - `IAM.hrisCompensation`: `app/dashboard/(dashboard)/hris/payroll/page.tsx`, `payroll/[id]/page.tsx`, `payroll/[id]/payslip/[employeeId]/page.tsx`, `payroll/settings/page.tsx`, `hris/salary/page.tsx`, `salary/insert/page.tsx`, `salary/[id]/page.tsx`
  - `IAM.hrisLoans`: `hris/loans/page.tsx`
  - `IAM.hrisAnalytics`: `hris/analytics/page.tsx`
  - `IAM.hrisRecruitment`: `hris/candidates`, `pipeline`, `psikotes`, `talent-pool`, `job-portal`, `live-monitoring` (`page.tsx`)
- Create: `frontend/src/lib/auth/home-for.ts` + `home-for.test.ts`
- Modify: `frontend/src/app/dashboard/(dashboard)/page.tsx`
- Test: `frontend/src/features/users/components/contracts/contract-card.test.tsx`

**Interfaces:**
- Consumes: respons `{ data, can_view_salary }` (Task 3); `IAM.hrisLoans`, `IAM.hrisAnalytics` (Task 1).
- Produces:
  - `useCanViewContractSalary(employeeId): boolean`;
  - prop `canViewSalary: boolean` di `ContractCard`, `ContractFormDialog`, `ContractAdminDialog`;
  - `homeFor(role: string, granted: readonly string[]): string`.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/auth/home-for.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { homeFor } from "./home-for";

describe("homeFor", () => {
  it("HR tanpa Rekrutmen/Analytics ke daftar karyawan", () => {
    expect(homeFor("hrd", ["hris.kepegawaian", "hris.kepegawaian.users", "hris.insights.reports"])).toBe(
      "/dashboard/employees"
    );
  });
  it("HR dengan Rekrutmen tetap ke dashboard rekrutmen", () => {
    expect(homeFor("hrd", ["hris.recruitment.candidates"])).toBe("/dashboard/rekrutmen");
  });
  it("peran lain tidak berubah", () => {
    expect(homeFor("finance_staff", [])).toBe("/dashboard/hris/payroll");
    expect(homeFor("pos", [])).toBe("/dashboard/pos");
    expect(homeFor("marketing", [])).toBe("/dashboard/rekrutmen");
  });
});
```

`frontend/src/features/users/components/contracts/contract-card.test.tsx`:

```tsx
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { EmployeeContractRow } from "../../api";
import { ContractCard } from "./contract-card";

afterEach(cleanup);

const contract = {
  id: "c1", employee_id: "e1", contract_number: "001/PKWT/X/2026", contract_type: "pkwt", status: "active",
  start_date: "2026-01-01", end_date: "2027-01-01", probation_end_date: null, sequence: 1,
  position_title: null, department_name: null, work_location: null, base_salary: "4800000.00",
  signed_at: null, signed_document_url: null, kemnaker_registered_at: null, compensation_amount: null,
  compensation_paid_at: null, terminated_reason: null, notes: null, created_by_name: null,
  created_at: "2026-01-01T00:00:00Z",
} as unknown as EmployeeContractRow;
const noop = () => {};
const props = { contract, actionPending: false, deletePending: false, onAction: noop, onEdit: noop,
  onAdmin: noop, onRenew: noop, onTerminate: noop, onDelete: noop };

it("tanpa akses Penggajian: tidak ada gaji pokok dan tombol PDF", () => {
  render(<ContractCard {...props} canViewSalary={false} />);
  expect(screen.queryByText("Gaji pokok")).toBeNull();
  expect(screen.queryByTitle("Unduh PDF surat perjanjian kerja")).toBeNull();
});

it("dengan akses Penggajian: gaji pokok dan PDF tampil", () => {
  render(<ContractCard {...props} canViewSalary />);
  expect(screen.getByText("Gaji pokok")).toBeTruthy();
  expect(screen.getByTitle("Unduh PDF surat perjanjian kerja")).toBeTruthy();
});
```

(Sesuaikan field fixture bila `EmployeeContractRow` punya field wajib lain. Cast `as unknown as` sudah menutup itu.)

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/auth/home-for.test.ts src/features/users/components/contracts/contract-card.test.tsx`
Expected: FAIL — `Cannot find module './home-for'`; kartu masih menampilkan "Gaji pokok".

- [ ] **Step 3: Implementasi**

`frontend/src/lib/auth/home-for.ts` (pindahkan `ROLE_HOME` dari `page.tsx`):

```ts
import { hasAnyIamMenuPrefix } from "@/lib/iam/match";
import { IAM } from "@/lib/iam/prefixes";

const ROLE_HOME: Record<string, string> = {
  purchasing_admin: "/dashboard/purchasing",
  purchasing_manager: "/dashboard/purchasing",
  purchasing_staff: "/dashboard/purchasing",
  pos: "/dashboard/pos",
  pos_supervisor: "/dashboard/pos",
  finance_staff: "/dashboard/hris/payroll",
};

const HR_ROLES = new Set(["hrd", "admin", "hiring_manager"]);

/**
 * Beranda per peran (EPIC-021). HR tanpa Rekrutmen/Analytics (akun
 * Non-Rekrutmen, keputusan 19-10-2026) ke daftar karyawan, bukan dashboard
 * rekrutmen yang tidak boleh dibukanya.
 */
export function homeFor(role: string, granted: readonly string[]): string {
  const home = ROLE_HOME[role] ?? "/dashboard/rekrutmen";
  if (
    home === "/dashboard/rekrutmen" &&
    HR_ROLES.has(role) &&
    !hasAnyIamMenuPrefix(granted, [...IAM.hrisRecruitment, ...IAM.hrisAnalytics])
  ) {
    return "/dashboard/employees";
  }
  return home;
}
```

`app/dashboard/(dashboard)/page.tsx`: hapus `ROLE_HOME`, ganti baris terakhir:

```tsx
  const granted = await loadGrantedMenuCodesForUser(user.id, user.role);
  redirect(homeFor(user.role, granted));
```

(import `homeFor` dari `@/lib/auth/home-for`, dan `loadGrantedMenuCodesForUser` dari modul yang sama dengan yang dipakai `require-user.ts`; lihat import di file itu.)

`features/users/api.ts`:
- Di `EmployeeContractRow`, ubah `base_salary`, `compensation_amount` menjadi opsional (`base_salary?: string | null;`, `compensation_amount?: string | null;`).
- `fetchEmployeeContracts` → `apiGet<{ data: EmployeeContractRow[]; can_view_salary?: boolean }>(...)`.

`features/users/queries.ts` — ganti `useEmployeeContracts`:

```ts
const employeeContractsQuery = (employeeId: string) => ({
  queryKey: usersQueryKeys.contracts(employeeId),
  queryFn: () => fetchEmployeeContracts(employeeId),
  enabled: !!employeeId,
});

export const useEmployeeContracts = (employeeId: string) =>
  useQuery({ ...employeeContractsQuery(employeeId), select: (res) => res.data ?? [] });

/** false bila akun tanpa akses Penggajian: angka gaji & PDF kontrak disembunyikan. */
export const useCanViewContractSalary = (employeeId: string) =>
  useQuery({ ...employeeContractsQuery(employeeId), select: (res) => res.can_view_salary !== false }).data ??
  false;
```

`contract-card.tsx`:
- Tambah `canViewSalary: boolean` di `ContractCardProps` + destructuring.
- Bungkus `<Field label="Gaji pokok">…</Field>` dan blok `contract.compensation_amount && (...)` dengan `canViewSalary && (...)`.
- Bungkus tombol PDF (`title="Unduh PDF surat perjanjian kerja"`) dengan `canViewSalary && (...)`.

`contract-form-dialog.tsx`:
- Tambah prop `canViewSalary: boolean`.
- Bungkus `<ContractField label="Gaji pokok (ops. — default gaji aktif)">…</ContractField>` dengan `canViewSalary && (...)`.
- Saat `!canViewSalary`, jangan kirim `base_salary` di payload. Cari tempat payload dibangun dan pakai `...(canViewSalary ? { base_salary: … } : {})`.

`contract-admin-dialog.tsx`:
- Tambah prop `canViewSalary: boolean`.
- Tombol lihat scan (baris 127-135, `contractSignedDocumentUrl`) hanya bila `canViewSalary`.
- Blok `compensation_amount` (baris 165) juga `canViewSalary && …`.

`employee-contracts-tab.tsx`: `const canViewSalary = useCanViewContractSalary(employeeId);` dan teruskan `canViewSalary={canViewSalary}` ke `ContractCard`, `ContractFormDialog`, dan `ContractAdminDialog`.

`employee-documents-tab.tsx`:
- `const canViewSalary = useCanViewContractSalary(employeeId);`
- Prop `action` tombol PDF kontrak menjadi `canViewSalary ? <Button …PDF/> : undefined`.
- Tombol lihat scan bertanda tangan diperlakukan sama.

Guard halaman — contoh untuk `hris/payroll/page.tsx`; ulangi pola yang sama untuk setiap halaman di daftar Files dengan prefix yang tertera:

```tsx
import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { PayrollPage } from "@/features/hris/payroll";

export default async function Page() {
  await requireIamPage(IAM.hrisCompensation, "/dashboard/me");
  return <PayrollPage />;
}
```

- Halaman dengan `params` tetap menerima `{ params }`, tinggal dibuat `async`.
- `live-monitoring` tetap mengekspor `metadata`.
- `hris/loans/page.tsx`: `await requireIamPage(IAM.hrisLoans, "/dashboard/me");`

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: `cd frontend && npx vitest run src/lib/auth src/features/users && npx tsc --noEmit -p . 2>&1 | tail -20`
Expected: semua PASS; `tsc` tanpa error baru.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(hris-ui): sembunyikan gaji & PDF kontrak tanpa akses Penggajian, guard halaman payroll/rekrutmen, beranda HR"
```

---

### Task 7: Seeder, skrip ops penugasan akun, dan dokumentasi epic

**Files:**
- Modify: `backend/database/seeders/paddy-hris-admins.js` (parseAdmins, seedAccount, header komentar)
- Create: `backend/database/sql/fixes/2026-10-21_hris_akses_per_akun.sql`
- Create: `docs/epics/EPIC-057-hris-akses-per-akun.md`
- Modify: `docs/epics/README.md` (baris registry EPIC-057)

**Interfaces:**
- Consumes: role codes Task 1.

- [ ] **Step 1: Tulis test yang gagal (parser seeder)**

Tambahkan di akhir `paddy-hris-admins.js` (sebelum pemanggilan `main()`, yang dibungkus `if (require.main === module)` bila belum):

```js
module.exports = { parseAdmins };
```

Test cepat (tidak di-commit sebagai file; cukup perintah):

```bash
cd backend/database && node -e '
const { parseAdmins } = require("./seeders/paddy-hris-admins.js");
const a = parseAdmins("A@x.com|Ani|hris_non_payroll;b@x.com|Budi");
if (a[0].role !== "hris_non_payroll" || a[1].role !== undefined) throw new Error(JSON.stringify(a));
let threw = false; try { parseAdmins("c@x.com|Cici|super_admin"); } catch { threw = true; }
if (!threw) throw new Error("role di luar daftar HR harus ditolak");
console.log("ok");'
```

Expected sebelum implementasi: FAIL (`role` undefined / tidak ada penolakan, atau `main()` langsung jalan saat di-require).

- [ ] **Step 2: Implementasi seeder**

- Konstanta: `const HR_ROLE_CODES = ["admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"];`
- `parseAdmins`: format `email|Nama[|role]`. Bila `role` diisi dan bukan anggota `HR_ROLE_CODES`, lempar `HRIS_ADMINS tidak valid: role "<x>" (pilih: …)`. Kembalikan `{ email, name, role }` (`role` undefined bila kosong).
- `seedAccount`:
  - cek akun lama memakai "punya salah satu `HR_ROLE_CODES`" (bukan hanya admin_hris);
  - role yang dipasang = `a.role` ?? role HR yang sudah dimiliki akun ?? `admin_hris`. Lookup `iam.roles` by code; error jelas bila role belum ada (`jalankan migrasi dulu`);
  - `DELETE … role_id <> $2` dan `INSERT` memakai id role terpilih.
- Header komentar: tambah poin "akun HR per email boleh memakai role turunan (hris_manager / hris_non_payroll / hris_non_rekrutmen_kpi); seeder tidak mengembalikannya ke admin_hris kecuali role diisi".

Run: perintah Step 1. Expected: `ok`.

- [ ] **Step 3: Skrip ops penugasan**

`backend/database/sql/fixes/2026-10-21_hris_akses_per_akun.sql`:

```sql
-- =============================================================================
-- Hak akses HRIS per akun email (EPIC-057, keputusan user 19-10-2026).
-- Jalankan SETELAH migrasi 20261019100000 dan backup:
--   psql "$U" -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-10-21_hris_akses_per_akun.sql
-- Peran aplikasi tetap hrd. Jangan simpan ulang akun ini di Manajemen User
-- dengan peran lain: mengganti peran aplikasi mengganti role IAM (syncIamRole).
-- =============================================================================
BEGIN;
CREATE TEMP VIEW hr_akses AS
SELECT * FROM (VALUES
  ('paddyhumanresources@gmail.com', 'hris_manager'),
  ('paddy.peopledevelopment@gmail.com', 'hris_non_payroll'),
  ('paddyhroperation@gmail.com', 'hris_non_rekrutmen_kpi')) v(email, role_code);

SELECT count(*) = 3 AS ok FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
JOIN iam.roles r ON r.code = a.role_code AND r.deleted_at IS NULL \gset
\if :ok
\else
  \echo 'Akun atau role tidak lengkap (harus 3) — tidak ada perubahan.'
  ROLLBACK;
  \quit
\endif

\echo 'Sebelum:'
SELECT a.email, string_agg(r.code, ', ') AS roles FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
LEFT JOIN iam.user_roles ur ON ur.user_id = u.id
LEFT JOIN iam.roles r ON r.id = ur.role_id GROUP BY a.email ORDER BY a.email;

DELETE FROM iam.user_roles ur USING hr_akses a, auth.users u
WHERE lower(u.email) = a.email AND ur.user_id = u.id;
INSERT INTO iam.user_roles (user_id, role_id, is_primary)
SELECT u.id, r.id, true FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
JOIN iam.roles r ON r.code = a.role_code;

\echo 'Sesudah:'
SELECT a.email, r.code, cu.role AS app_role FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
JOIN iam.user_roles ur ON ur.user_id = u.id
JOIN iam.roles r ON r.id = ur.role_id
JOIN configuration.users cu ON cu.id = u.id ORDER BY a.email;
COMMIT;
```

Uji pada DB test:

Run: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-10-21_hris_akses_per_akun.sql`
Expected, salah satu dari:
- bila ketiga akun tidak ada di DB test: `Akun atau role tidak lengkap (harus 3) — tidak ada perubahan.`, tanpa perubahan;
- bila DB test berisi salinan akun itu: blok "Sesudah" menampilkan 3 baris dengan role sesuai tabel dan `app_role = hrd`.

Lalu uji guard: ubah sementara satu email di view menjadi `tidak-ada@contoh.invalid` dan jalankan lagi. Harus muncul pesan "tidak lengkap" dan tidak ada perubahan. Kembalikan setelahnya; jangan di-commit.

- [ ] **Step 4: Dokumen epic**

`docs/epics/EPIC-057-hris-akses-per-akun.md`:

```markdown
# EPIC-057: Hak Akses HRIS per Akun Email

status: on-progress
environment: dev
retries: 0

## Goal
Tiga akun HR mendapat hak akses masing-masing (Manager seluruh HRIS; Staff Non-Payroll; Staff Non-Rekrutmen & KPI), dengan menu, halaman, dan API yang konsisten.

## Tasks
Lihat docs/superpowers/plans/2026-10-19-hris-akses-per-akun.md (Task 1–8).

## Acceptance Criteria
- paddyhumanresources@gmail.com membuka seluruh menu HRIS.
- paddy.peopledevelopment@gmail.com tidak melihat Penggajian; Pinjaman tetap; kontrak tampil tanpa angka gaji dan tanpa PDF; slip gaji orang lain tertutup.
- paddyhroperation@gmail.com tidak melihat Rekrutmen, Kinerja, maupun Analytics; beranda ke daftar karyawan; KPI/Review/Task Departemen/Logbook hanya data sendiri.
- admin_hris dan peran lain tidak berubah.

## Automation Log
- 2026-10-19: rencana ditulis; keputusan user & ruling R1–R7 tercatat di rencana.
```

Lalu tambahkan baris EPIC-057 ke tabel di `docs/epics/README.md`, mengikuti format baris EPIC-056.

- [ ] **Step 5: Commit**

```bash
git add backend/database/seeders/paddy-hris-admins.js backend/database/sql/fixes/2026-10-21_hris_akses_per_akun.sql docs/epics/EPIC-057-hris-akses-per-akun.md docs/epics/README.md
git commit -m "chore(hris): seeder & skrip ops akses per akun HR, EPIC-057"
```

---

### Task 8: Uji di stack salinan DB, lalu (dengan persetujuan) live

**Files:** tidak ada perubahan kode. Hasil uji masuk `docs/epics/EPIC-057-hris-akses-per-akun.md` (Automation Log) dan screenshot ke `.share/epic057/`.

- [ ] **Step 1: Suite penuh**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... > /tmp/epic057-go.txt 2>&1; grep -c -- '--- FAIL' /tmp/epic057-go.txt; cd ../frontend && npx vitest run 2>&1 | tail -5`
Expected: jumlah FAIL = baseline (27); daftar nama sama dengan di main; vitest PASS.

- [ ] **Step 2: Stack salinan DB**

Ulangi pola EPIC-056:
- dump live ke `ui-db` (port 55521);
- bangun `ui-api`/`ui-web` dari branch, di `127.0.0.1:8148`, network `ui-net`;
- jalankan migrasi lalu skrip ops di **salinan**. Output "Sesudah" harus 3 baris dengan role yang benar dan `app_role = hrd`.
- **Jangan** ubah password akun asli. Untuk login uji, buat 3 akun uji di salinan saja (`hrd` + masing-masing role baru) dengan password sementara, dan hapus bersama stack.

- [ ] **Step 3: Cek per akun (Playwright, di salinan)**

| Akun uji (role) | Harus terlihat | Harus tertutup |
|---|---|---|
| hris_manager | Semua menu HRIS; gaji & PDF kontrak; slip semua karyawan | — |
| hris_non_payroll | Kepegawaian (Karyawan, Kontrak tanpa gaji/PDF, **Pinjaman** + approve), Absensi, Cuti, Rekrutmen, Kinerja, Laporan | Menu Penggajian; `/dashboard/hris/payroll` → `/dashboard/me`; `GET /api/hris/payslips?employee_id=<lain>` kosong |
| hris_non_rekrutmen_kpi | Kepegawaian lengkap (gaji & PDF tampil), Penggajian, Absensi, Cuti, Laporan HRIS; beranda → `/dashboard/employees` | Menu Rekrutmen, Kinerja, Analytics; `/dashboard/hris/candidates` → `/dashboard/me`; `/api/dashboard/stats` 403 |

Simpan screenshot per baris di `.share/epic057/`; console tanpa error baru.

- [ ] **Step 4: Tunjukkan hasil ke user & minta persetujuan live**

Laporkan tabel Step 3 + screenshot. **Berhenti di sini** sampai user menyetujui.

- [ ] **Step 5: Live (setelah disetujui)**

Urutannya:
1. Backup: `pg_dump -Fc` live ke `~/Desktop/paddydev-backups/paddydev-db-before-epic057-<ts>.dump`.
2. Merge ke main & push `paddydev`.
3. `go run ./cmd/migrate -apply` ke live (`U` dari `~/.config/paddydev/api.env`, host `127.0.0.1:5485`).
4. Tunjukkan output "Sebelum" skrip ops ke user, lalu jalankan skrip ops.
5. Ganti container API & web dengan pola swap + rollback.
6. Cek ulang: login `paddyhroperation@gmail.com` (password yang diketahui) dan verifikasi baris ketiga tabel Step 3. Dua akun lain diverifikasi lewat query `iam.user_roles` dan grant menu, karena passwordnya tidak dipegang.
7. Catat di Automation Log EPIC-057, set `status: ready-for-qa`, commit docs.
