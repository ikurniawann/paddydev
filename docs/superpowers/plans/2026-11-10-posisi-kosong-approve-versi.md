# Posisi Kosong & Persetujuan Versi Bagan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR sees which org-chart slots have no holder, and can approve one chart version as the official one.

**Architecture:**
- Vacancy is computed per request, on top of the EPIC-101 matcher (`domain.MatchOrgChart`). Ambiguous employees now carry their candidate node codes.
- Approval is one guarded UPDATE pair in a transaction, plus an audit row. A partial unique index enforces a single approved version.
- The frontend adds a version bar (selector + approve dialog) and a "Posisi Kosong" tab to the existing Struktur Organisasi page.

**Tech Stack:** Go 1.26 (pgx, `internal/platform/xlsx`, `audit`), PostgreSQL 16, Next.js 16.4.2 + React Query + vitest.

**Spec:** `docs/epics/EPIC-102-posisi-kosong-approve-versi.md` (rulings R1–R9). Builds on `docs/epics/EPIC-101-bagan-organisasi-draft6.md`.

## Global Constraints

- No change to `hris.employees`, `hris.positions`, `hris.departments`, `configuration.branches`, `reporting_to`, or approval flows (R1).
- `hris.org_chart_nodes.status` is never written (R2).
- Read routes use `h.withMenu(iam.HrisOrganization, …)`. The approve route uses `h.withAction("approve", iam.HrisOrganization, …)`. No new menu (R8).
- All user-facing copy is Indonesian, matching EPIC-101 wording ("Posisi Kosong", "Mungkin terisi", "Rencana", "Dibekukan", "Terisi", "Setujui versi").
- Before writing frontend code, read the relevant guide in `frontend/node_modules/next/dist/docs/` (`frontend/AGENTS.md`).
- Go tests run against a migrated test DB: `TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55499/<db>?sslmode=disable`. Container `paddydev-sdd-testdb`; create a fresh DB and run `go run ./cmd/migrate -apply`. If `go` is missing on the host, use `~/.local/go/bin` or the `golang:1.26-alpine` container (memory `hris-epic098-impor-nip-atasan`).
- Production is not touched by this plan. Deploy needs separate user permission (memory `alur-migrasi-production`).

## Review Focus

1. An employee whose title matches several slots must not leave those slots looking "Kosong". Expect "Mungkin terisi" with their name. Test: T1 `TestOrgVacanciesAmbiguousIsMaybe`.
2. Approving while another version is already approved archives the old one in the same transaction. A second concurrent approve gets 409, not two approved rows. Test: T3 `TestOrgChartApproveArchivesPrevious`, `TestOrgChartApproveTwiceConflict`.
3. A version with BR errors (e.g. two roots) cannot be approved, and nothing changes. Test: T3 `TestOrgChartApproveBlockedByErrors`.
4. After a newer draft is imported, the default view stays on the approved version. Test: T3 `TestOrgChartDefaultVersionIsApproved`.
5. Group nodes, frozen and planned slots are never counted as "Kosong". Filtering `region=` must not drop HQ nodes whose region is empty when no region is chosen. Test: T1 `TestOrgVacanciesStates`, T2 `TestOrgChartVacancyAPI`.

---

### Task 1: Domain — candidates, vacancy state, group summary

**Files:**
- Modify: `backend/internal/modules/hris/domain/org_chart.go`
- Test: `backend/internal/modules/hris/domain/org_chart_test.go`

**Interfaces:**
- Produces:
  - `OrgMatch.Candidates []string` with JSON tag `candidate_codes,omitempty`. Set only in the ambiguous case (more than one candidate), sorted.
  - Vacancy state constants:
    ```go
    const (
        VacancyFilled  = "filled"
        VacancyEmpty   = "empty"
        VacancyMaybe   = "maybe"
        VacancyPlanned = "planned"
        VacancyFrozen  = "frozen"
    )
    ```
  - `type OrgVacancy struct { State string; Holders int; MaybeHolders []string }`
  - `func OrgVacancies(nodes []OrgNode, g OrgGap) map[string]OrgVacancy`: keyed by node code, position nodes only. Uses the R2 order. `MaybeHolders` holds the employee names (sorted) from `g.Matches` whose `Candidates` contain the code.
  - `type OrgVacancyGroup struct { Division, Region string; Positions, Filled, Empty, Maybe, Planned, Frozen int }`
  - `func GroupVacancies(nodes []OrgNode, v map[string]OrgVacancy) []OrgVacancyGroup`: one row per (DivisionCode, Region). Sorted by Division, then Region, with `""` first.

- [ ] **Step 1: Write the failing tests**
  - `TestMatchOrgChartAmbiguousCandidates`: two `Shopkeeper` nodes `RTL-010` (stores `PVJ1`) and `RTL-011` (stores `PVJ2`), one employee titled Shopkeeper with no store. The match has `NodeCode == ""` and `Candidates == []string{"RTL-010", "RTL-011"}`.
  - `TestOrgVacanciesStates` uses nodes `G` (group), `A` (active, 1 holder), `B` (active, 0), `C` (planned, 0), `D` (frozen, 1 holder), `E` (vacant, 0), `F` (planned, 1 holder). Expect:
    - `G` absent;
    - `A` filled, Holders 1;
    - `B` empty;
    - `C` planned;
    - `D` frozen;
    - `E` empty;
    - `F` filled.
  - `TestOrgVacanciesAmbiguousIsMaybe`: the two Shopkeeper nodes from the first test are both `maybe`, with `MaybeHolders == []string{"Sinta"}`. A third unrelated empty node stays `empty`.
  - `TestGroupVacancies`: nodes in `RTL/Bandung`, `RTL/Bandung`, `RTL/""` and `FIN/""`. Expect order `FIN/""`, `RTL/""`, `RTL/Bandung`, with Positions and state counts per group.
- [ ] **Step 2: Run them to see them fail**
  Run `cd backend && go test ./internal/modules/hris/domain -run 'OrgVacanc|GroupVacancies|AmbiguousCandidates' -v`. Expect a compile failure: `undefined: OrgVacancies`.
- [ ] **Step 3: Implement**
  Set `m.Candidates` in the `default:` branch of `MatchOrgChart`. Add `OrgVacancies` and `GroupVacancies` beside it. Pure functions, no I/O.
- [ ] **Step 4: Run the domain package**
  Run `go test ./internal/modules/hris/domain -count=1`. Expect PASS, including the existing `org_chart_test.go`.
- [ ] **Step 5: Commit**
  `feat(hris): keadaan posisi bagan dan kandidat karyawan ambigu (EPIC-102)`

### Task 2: API Posisi Kosong (JSON + Excel)

**Files:**
- Create: `backend/internal/modules/hris/org_chart_vacancy.go`
- Modify: `backend/internal/modules/hris/routes.go` (beside the org-chart routes, line ~46)
- Modify: `frontend/src/lib/go-routes.generated.json` (regenerated, `backend/Makefile:36`)
- Test: `backend/internal/modules/hris/org_chart_integration_test.go`

**Interfaces:**
- Consumes: `OrgVacancies`, `GroupVacancies` (T1); `(*Service).orgGap(ctx, label)`; `orgNodeView`; `orgGapWorkbook` style; `writeFile`, `exportFile`.
- Produces: `GET /api/hris/org-chart/vacancy?version=&division=&region=&state=&format=xlsx`.
  - Response `data`:
    ```
    { version, summary: {positions, filled, empty, maybe, planned, frozen}, groups: [{division, region, positions, filled, empty, maybe, planned, frozen}], rows: [...] }
    ```
  - Each row:
    ```
    { code, title, division_code, division_name, region, store_codes, parent_code, parent_title, state, holders, maybe_holders }
    ```
  - `rows` holds every position whose state is not `filled`, in chart `sort_order`. `state=` narrows rows to one state.
  - `division=` and `region=` filter nodes before counting. An empty filter means all, and region `""` nodes are kept when no region is given.
  - `parent_title` is the effective parent's title (group nodes skipped), or `""`.
  - Excel file `Posisi Kosong <label>.xlsx` has two sheets:
    - `Ringkasan`: Divisi, Region, Posisi, Terisi, Kosong, Mungkin terisi, Rencana, Dibekukan.
    - `Posisi`: Kode, Jabatan, Divisi, Region, Toko, Atasan Bagan, Keadaan, Kemungkinan Pemegang.
  - Keadaan labels: `empty` "Kosong", `maybe` "Mungkin terisi", `planned` "Rencana", `frozen` "Dibekukan".

- [ ] **Step 1: Write the failing test** `TestOrgChartVacancyAPI` in `org_chart_integration_test.go`, on the seeded Draft 6. The test DB may hold other employees, so assert relative counts, as `TestOrgChartAPI` does:
  - `none` user → 403.
  - `hr` user → 200, with `summary.positions == 149`.
  - `filled + empty + maybe + planned + frozen == 149`.
  - `?division=RTL&region=Makassar` → `groups` has one row (RTL/Makassar, positions 4). Every row code is in `RTL-053..RTL-056`.
  - `?state=planned` → every row has `"state":"planned"`.
  - `?format=xlsx` → Content-Type `xlsx.ContentType`, and the workbook has sheets `Ringkasan` and `Posisi`.
  - Add the Perfumer at PLBM fixture from `TestOrgChartAPI` (branch `ODOO-PLBM`, title `Perfumer Staff`, primary placement). Then `RTL-023` is absent from `rows`, and `summary.filled` is one more than before.
- [ ] **Step 2: Run it to see it fail**
  Run `go test ./internal/modules/hris -run TestOrgChartVacancyAPI -count=1`. Expect 404 or route-not-found.
- [ ] **Step 3: Implement `func (h handlers) orgChartVacancy(w http.ResponseWriter, r *http.Request, _ *auth.User) error`** in `org_chart_vacancy.go`, plus `orgVacancyWorkbook(...) (*exportFile, error)`. Register `r("GET /api/hris/org-chart/vacancy", h.withMenu(iam.HrisOrganization, h.orgChartVacancy))`.
- [ ] **Step 4: Regenerate routes**
  Run `cd backend && go run ./cmd/api -routes > ../frontend/src/lib/go-routes.generated.json`. Then run `cd frontend && npx vitest run src/lib/backend-routes.test.ts`. Expect PASS: the `/api/hris/org-chart` prefix is already registered.
- [ ] **Step 5: Run** `go test ./internal/modules/hris -run 'OrgChart' -count=1`. Expect PASS.
- [ ] **Step 6: Commit**
  `feat(hris): API posisi kosong bagan per divisi/region + Excel (EPIC-102)`

### Task 3: Migration, approve API, default version

**Files:**
- Create: `backend/database/migrations/deltas/20261116090000_org_chart_approval.sql`
- Modify: `backend/internal/modules/hris/org_chart.go` (`orgVersion`, `orgVersions`, `orgVersionOf`)
- Create: `backend/internal/modules/hris/org_chart_approve.go`
- Modify: `backend/internal/modules/hris/routes.go`, `frontend/src/lib/go-routes.generated.json`
- Test: `backend/internal/modules/hris/org_chart_approve_integration_test.go`

**Interfaces:**
- Migration:
  - Add `approved_at timestamptz` and `approved_by_name text` to `hris.org_chart_versions`.
  - Add `CREATE UNIQUE INDEX IF NOT EXISTS uq_org_chart_versions_one_approved ON hris.org_chart_versions ((true)) WHERE status = 'approved'`.
  - No data change: Draft 6 stays `draft`.
- `orgVersion` gains:
  ```go
  ValidFrom      *string    `json:"valid_from"`     // YYYY-MM-DD
  ApprovedAt     *time.Time `json:"approved_at"`
  ApprovedByName *string    `json:"approved_by_name"`
  ```
- `orgVersionOf(ctx, q, "")` returns the `approved` version when one exists, else the latest import (R7). A named label works as before.
- `POST /api/hris/org-chart/versions/{id}/approve`
  - Body (optional, `optionalForm(r)`): `{ "valid_from": "YYYY-MM-DD", "reason": "..." }`.
  - Responses:
    - 200: `data` = the approved `orgVersion`.
    - 400 "Tanggal berlaku tidak valid": bad date.
    - 400 "Bagan masih punya N kesalahan struktur — perbaiki dulu sebelum disetujui": BR errors exist, with details = the `[]OrgIssue` errors.
    - 404 "Versi bagan tidak ditemukan".
    - 409 "Versi ini sudah tidak berstatus draft": not draft, or lost a race (also on unique-index violation `23505`).
  - Inside one `s.repo.InTx`:
    1. `UPDATE … SET status='archived' WHERE status='approved' AND id <> $1 RETURNING label`.
    2. `UPDATE … SET status='approved', valid_from=$2, approved_at=now(), approved_by_name=$3 WHERE id=$1 AND status='draft'`. Zero rows → 409.
    3. `audit.Write` with the R9 entry, `ActorID: u.ID`, `ActorName: &u.FullName`, `.WithRequest(r)`, reason.
- Route: `r("POST /api/hris/org-chart/versions/{id}/approve", h.withAction("approve", iam.HrisOrganization, h.orgChartApprove))`.

- [ ] **Step 1: Write the failing tests** in `org_chart_approve_integration_test.go`:
  - `TestOrgChartApprove`:
    - A user granted only `read` on `hris.organization` (use `h.staffGrants`) gets 403.
    - `hr` posts `{"valid_from":"2026-11-12"}` → 200, with `status=approved`, `valid_from=2026-11-12`, and `approved_by_name` set.
    - One audit row has action `hris.org_chart.version.approve`.
    - `GET /versions` shows Draft 6 as approved.
  - `TestOrgChartApproveBlockedByErrors`: insert version `Draft X` with two root nodes → 400 "kesalahan struktur". Status stays `draft` and no audit row is written.
  - `TestOrgChartApproveArchivesPrevious`: approve Draft 6, then insert and approve a valid one-node version `Draft 7` → Draft 6 becomes `archived` and Draft 7 `approved`. The audit `after` names the archived label.
  - `TestOrgChartApproveTwiceConflict`: approving Draft 6 twice → the second call gets 409.
  - `TestOrgChartDefaultVersionIsApproved`: approve Draft 6, then insert a newer draft `Draft 7`. Both `GET /tree` and `GET /vacancy` without `?version=` return `version.label == "Draft 6"`.
  - Bad date `{"valid_from":"12-11-2026"}` → 400 "Tanggal berlaku tidak valid".
- [ ] **Step 2: Run them to see them fail**
  Run `go test ./internal/modules/hris -run 'OrgChartApprove|DefaultVersion' -count=1`. Expect failure.
- [ ] **Step 3: Implement** the migration, the `orgVersion` fields and query, the `orgVersionOf` default, and `func (h handlers) orgChartApprove(w http.ResponseWriter, r *http.Request, u *auth.User) error`. Validate with `domain.ValidateOrgChart(domainNodes(nodes))`. Read the date with `domain.ParseDate`. Default the date to `domain.TodayWIB(h.svc.now())`.
- [ ] **Step 4: Regenerate routes** (same command as T2 Step 4), then run `go test ./internal/modules/hris/... -count=1`. Expect PASS with no regressions.
- [ ] **Step 5: Commit**
  `feat(hris): persetujuan versi bagan + versi bawaan = disetujui (EPIC-102)`

### Task 4: Frontend — version bar, approve dialog, Posisi Kosong tab

**Files:**
- Modify: `frontend/src/features/hris/org-chart/{api.ts,query-keys.ts,types.ts}`
- Modify: `frontend/src/features/hris/org-chart/components/{org-chart-page.tsx,chart-tree.tsx,chart-gap.tsx}`
- Create: `frontend/src/features/hris/org-chart/components/version-bar.tsx`, `chart-vacancy.tsx`
- Test: `version-bar.test.tsx`, `chart-vacancy.test.tsx`; update `chart-tree.test.tsx` and `chart-gap.test.tsx` for the new `version` prop.

**Interfaces:**
- Consumes: the T2 and T3 APIs.
- Produces:
  - `types.ts`:
    - `ChartVersion` = `{ id, label, status: "draft" | "approved" | "archived", valid_from, approved_at, approved_by_name, nodes }`.
    - `VacancyState` = `"empty" | "maybe" | "planned" | "frozen"`.
    - `ChartVacancyRow`, `ChartVacancyGroup`, `ChartVacancyResponse` mirror T2.
  - `api.ts`:
    - `fetchOrgChartVersions()`.
    - `fetchOrgChartTree(filter, version)`, `fetchOrgChartValidation(version)` and `fetchOrgChartGap(version)` gain `version: string` ("" = default; sent only when non-empty).
    - `fetchOrgChartVacancy(version, filter: ChartTreeFilter & { state: string })`.
    - `orgChartVacancyXlsxUrl(version, filter): string`.
    - `approveOrgChartVersion(id: string, body: { valid_from: string; reason?: string })` via `apiPost`.
  - `query-keys.ts`:
    - `versions()`.
    - `tree(version, division, region)`, `validation(version)`, `gap(version)`.
    - `vacancy(version, division, region, state)`.
    - Everything stays under `all` so one invalidate refreshes all of it.
  - `OrgChartPage`:
    - Holds `version` state (`""` = default).
    - Tabs become `Struktur Aktual`, `Bagan`, `Posisi Kosong`, `Selisih`.
    - `<VersionBar version onChange />` renders above the chart tabs (not on Struktur Aktual).
    - `ChartTree`, `ChartGap` and `ChartVacancy` take `{ version: string }`.
  - `VersionBar`:
    - The shown version is `version` when set. Otherwise it is the `approved` one, else the first in the list (the API sorts by latest import), which matches the server default (R7).
    - A Select of versions labelled with a status badge: Draft (yellow), Disetujui (green), Arsip (gray).
    - Under it: "Disetujui <approved_by_name> · berlaku sejak <valid_from>" when the shown version is approved.
    - Button **Setujui versi** only when the shown version is `draft`. It opens a dialog with:
      - a date input (default today, local date);
      - an optional Alasan textarea;
      - the validation error count from `fetchOrgChartValidation(version)`.
    - **Setujui** is disabled while errors > 0, showing "Perbaiki N kesalahan struktur dulu".
    - On success: toast "Versi <label> disetujui", then `invalidateQueries({ queryKey: orgChartQueryKeys.all })`.
    - On error: toast with the server message.
  - `ChartVacancy`:
    - Summary tiles Posisi / Terisi / Kosong / Mungkin terisi / Rencana.
    - Filters: Divisi, Region, Keadaan.
    - A group table and a rows table. "Mungkin terisi" rows list `maybe_holders`.
    - An "Unduh Excel" link to `orgChartVacancyXlsxUrl`.

- [ ] **Step 1: Write failing tests**
  - `chart-vacancy.test.tsx`, with `../api` mocked like `chart-gap.test.tsx`:
    - Renders summary values (e.g. `Kosong 120`).
    - A row `RTL-010 Shopkeeper` with "Mungkin terisi" and "Sinta".
    - The Excel link href contains `/api/hris/org-chart/vacancy` and `format=xlsx`.
  - `version-bar.test.tsx`:
    - The draft version shows **Setujui versi**; the approved version does not, and shows "berlaku sejak 2026-11-12".
    - With validation `errors: [1 item]`, the dialog's **Setujui** is disabled and shows "Perbaiki 1 kesalahan struktur dulu".
    - With 0 errors, clicking **Setujui** calls `approveOrgChartVersion("v1", { valid_from: <date input value>, reason: "" })`.
- [ ] **Step 2: Run them to see them fail**
  Run `cd frontend && npx vitest run src/features/hris/org-chart`. Expect failure: missing modules.
- [ ] **Step 3: Implement** the files above. Thread `version` into the existing tree and gap queries.
- [ ] **Step 4: Gate frontend**
  Run `npx vitest run` (full suite, as EPIC-101 learned), `npx tsc --noEmit` (use `NODE_OPTIONS=--max-old-space-size=8192` if it is killed), and `npx eslint src/features/hris/org-chart`. Expect all green.
- [ ] **Step 5: Commit**
  `feat(hris): pemilih versi, setujui versi, dan tab Posisi Kosong (EPIC-102)`

### Task 5: Gate, staging QA, epic log

**Files:**
- Modify: `docs/epics/EPIC-102-posisi-kosong-approve-versi.md` (tasks checked, Automation Log), `docs/epics/README.md` (status)

- [ ] **Step 1: Full backend gate**
  Run gofmt (`gofmt -l ./internal ./cmd` prints nothing), then `go vet ./...` and `go test ./internal/... -count=1` on a freshly migrated test DB. Record the counts in the log.
- [ ] **Step 2: Deploy to staging**
  Check `df -h /` first (memory `disk-host-penuh-20261109`). Back up the staging DB to `/mnt/hdd1/backup/paddydev/staging-before-e102-<stamp>.dump`. Then run `bash tools/deploy-paddydev.sh`, and only after Step 1 passes.
- [ ] **Step 3: QA on staging with an HR QA account** (staging accounts are allowed):
  - Posisi Kosong totals match `GET /vacancy`.
  - Filter Makassar shows 4.
  - The Excel file opens.
  - Approve Draft 6 → badge Disetujui, and an audit row exists.
  - A read-only account cannot approve.
  - Afterwards, restore Draft 6 to `draft` from the staging backup or a scoped SQL, so HR approves it themselves on production.
  - Delete the QA accounts.
- [ ] **Step 4: Epic log and status**
  Check the tasks off. Write the gate, QA, and staging image to the Automation Log, and set the status to `review`. Commit `docs: EPIC-102 selesai, hasil gate dan QA staging`.
