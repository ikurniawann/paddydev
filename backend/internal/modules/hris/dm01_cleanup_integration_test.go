package hris

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// EPIC-083 (DM-01): the cleanup scripts in backend/database/sql/fixes run
// inside the fixture transaction, which the harness rolls back.

func dm01Script(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "sql", "fixes", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// dm01Preview runs the preview's single SELECT: the psql meta-commands and
// the BEGIN READ ONLY / ROLLBACK lines psql needs are dropped.
func (h *harness) dm01Preview() []string {
	h.t.Helper()
	var body []string
	for _, line := range strings.Split(dm01Script(h.t, "2026-11-03_dm01_preview.sql"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `\`) || trimmed == "BEGIN READ ONLY;" || trimmed == "ROLLBACK;" {
			continue
		}
		body = append(body, line)
	}
	sql := strings.TrimRight(strings.TrimSpace(strings.Join(body, "\n")), ";")
	rows, err := h.tx.Query(context.Background(), sql)
	if err != nil {
		h.t.Fatalf("preview: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatalf("preview: %v", err)
	}
	return out
}

// dm01Lines are the preview lines with prefix, the prefix removed.
func dm01Lines(lines []string, prefix string) []string {
	var out []string
	for _, l := range lines {
		if rest, ok := strings.CutPrefix(l, prefix+"\t"); ok {
			out = append(out, rest)
		}
	}
	return out
}

// dm01Count is the value of "<prefix>\t<key>\t<n>"; "" when absent.
func dm01Count(lines []string, prefix, key string) string {
	for _, l := range dm01Lines(lines, prefix) {
		if n, ok := strings.CutPrefix(l, key+"\t"); ok {
			return n
		}
	}
	return ""
}

// dm01Core runs the core with the approved counts taken from a fresh
// preview, the way tools/ops/dm01-cleanup.sh passes them (dm01.expect_<table>).
func (h *harness) dm01Core() {
	h.t.Helper()
	h.dm01Expect(h.dm01Preview(), nil)
	h.exec(dm01Script(h.t, "2026-11-03_dm01_cleanup_core.sql"))
}

// dm01Expect sets dm01.expect_<table> for the transaction from the preview's
// HAPUS lines, plus bump[table].
func (h *harness) dm01Expect(preview []string, bump map[string]int) {
	h.t.Helper()
	for _, table := range dm01Tables {
		n, err := strconv.Atoi(dm01Count(preview, "HAPUS", table))
		if err != nil {
			h.t.Fatalf("preview HAPUS %s: %v", table, err)
		}
		h.exec(`SELECT set_config('dm01.expect_' || $1, $2, true)`, table, strconv.Itoa(n+bump[table]))
	}
}

var dm01Tables = []string{"attendance_requests", "attendance", "leaves", "overtime_requests", "business_trips", "loans"}

const dm01Checksum = `SELECT count(*)::text || ':' || COALESCE(md5(string_agg(lb::text, '' ORDER BY lb.id)), '')
	FROM hris.leave_balances lb`

func TestDM01Cleanup(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", nil, false) // notify_hrd writes to every users.role = 'hrd'
	e := h.employee(emp{name: "DM01 Uji"})
	f := func(name string) string { return e + "/" + name }

	h.exec(`INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_used, sick_leave_used)
		VALUES ($1, 2026, 12, 3, 1)
		ON CONFLICT (employee_id, year) DO UPDATE SET annual_leave_total = 12, annual_leave_used = 3, sick_leave_used = 1`, e)

	att := func(date, in, out string) string {
		return h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, clock_in_photo_url, clock_out_photo_url)
			VALUES ($1, $2::date, $2::date + time '08:00', 'present', NULLIF($3, ''), NULLIF($4, '')) RETURNING id::text`,
			e, date, in, out)
	}
	att0915 := att("2026-09-15", "attendance/"+f("0915-in.jpg"), "")
	att1020 := att("2026-10-20", "attendance/"+f("1020-in.jpg"), "attendance/"+f("1020-out.jpg"))
	att1021 := att("2026-10-21", "attendance/"+f("1021-in.jpg"), "")
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date, status, closed_at)
		VALUES (2026, 9, '2026-09-01', '2026-09-30', '2026-10-03', 'closed', now())`)
	// The guard is live: a plain delete in the closed period is refused.
	h.exec("SAVEPOINT dm01_guard")
	if _, err := h.tx.Exec(context.Background(), `DELETE FROM hris.attendance WHERE id = $1`, att0915); err == nil ||
		!strings.Contains(err.Error(), "sudah di-closing") {
		t.Fatalf("closed-period delete = %v, want the guard error", err)
	}
	h.exec("ROLLBACK TO SAVEPOINT dm01_guard")

	request := func(date, attendanceID, file string) string {
		return h.scalar(`INSERT INTO hris.attendance_requests (employee_id, date, kind, clock_in, reason, attendance_id, attachment_path)
			VALUES ($1, $2, 'correction', '08:00', 'Koreksi jam masuk', $3, NULLIF($4, '')) RETURNING id::text`,
			e, date, attendanceID, file)
	}
	request("2026-10-19", att1021, "attendance-requests/"+f("1019.jpg"))
	req1022 := request("2026-10-22", att1020, "")

	leave := func(start, end, status, file string) string {
		return h.scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, attachment_url)
			VALUES ($1, 'annual', $2, $3, 1, 'Cuti uji DM-01', $4, NULLIF($5, '')) RETURNING id::text`, e, start, end, status, file)
	}
	leave("2026-10-01", "2026-10-03", "approved", "leave-attachments/"+f("1001.jpg"))
	lv1822 := leave("2026-10-18", "2026-10-22", "approved", "")
	lv1025 := leave("2026-10-25", "2026-10-25", "pending", "")

	overtime := func(date string, files ...string) string {
		return h.scalar(`INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, reason, attachments)
			VALUES ($1, $2, '18:00', '20:00', 2, 'Lembur uji', $3) RETURNING id::text`, e, date, files)
	}
	shared := "overtime-attachments/" + f("shared.pdf")
	overtime("2026-10-20", "overtime-attachments/"+f("1020.pdf"), shared)
	ot1021 := overtime("2026-10-21", shared)

	loan := func(requested, status string, remaining *int, files, collateral []string) string {
		return h.scalar(`INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
			remaining_balance, status, requested_date, attachments, collateral_attachments)
			VALUES ($1, 'kasbon', 1000000, 2, 500000, $2, $3, $4::timestamptz, $5, $6) RETURNING id::text`,
			e, remaining, status, requested, files, collateral)
	}
	zero, half := 0, 500000
	none := []string{}
	loan("2026-10-01 09:00+07", "approved", &zero, []string{"loan-attachments/" + f("paid.pdf")},
		[]string{"loan-attachments/" + f("paid-col.pdf")})
	lnRunning := loan("2026-10-01 09:00+07", "approved", &half, []string{"loan-attachments/" + f("running.pdf")}, none)
	loan("2026-10-01 09:00+07", "pending", &half, none, none)
	loan("2026-10-01 09:00+07", "rejected", &half, none, none)
	lnLate := loan("2026-10-25 09:00+07", "approved", &half, none, none)

	checksum := h.scalar(dm01Checksum)
	balance := h.scalar(`SELECT concat_ws('/', annual_leave_total, annual_leave_used, sick_leave_used, updated_at)
		FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, e)
	notifications := func() string { return h.scalar(`SELECT count(*)::text FROM public.notifications`) }
	hrNotifications := func() string {
		return h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1`, hr.UserID)
	}
	notesBefore, hrBefore := notifications(), hrNotifications()

	preview := h.dm01Preview()
	want := map[string]string{"attendance_requests": "1", "attendance": "2", "leaves": "1",
		"overtime_requests": "1", "business_trips": "0", "loans": "3"}
	for _, table := range dm01Tables {
		if got := dm01Count(preview, "HAPUS", table); got != want[table] {
			t.Errorf("preview HAPUS %s = %q, want %s\n%s", table, got, want[table], strings.Join(preview, "\n"))
		}
	}
	wantKept := map[string]string{"attendance_requests": "1", "attendance": "1", "leaves": "2",
		"overtime_requests": "1", "business_trips": "0", "loans": "2"}
	for _, table := range dm01Tables {
		if got := dm01Count(preview, "TETAP", table); got != wantKept[table] {
			t.Errorf("preview TETAP %s = %q, want %s", table, got, wantKept[table])
		}
	}
	if got := dm01Count(preview, "STATUS", "loans\thapus\tapproved"); got != "1" {
		t.Errorf("preview STATUS loans hapus approved = %q, want 1", got)
	}
	if got := dm01Count(preview, "PENGECUALIAN", "leaves\tmelintasi 21 Okt"); got != "1" {
		t.Errorf("preview PENGECUALIAN leaves = %q, want 1", got)
	}
	if got := dm01Count(preview, "PENGECUALIAN", "loans\tberjalan (approved, sisa > 0)"); got != "1" {
		t.Errorf("preview PENGECUALIAN loans = %q, want 1", got)
	}
	if got := dm01Count(preview, "CHECKSUM", "leave_balances"); got != strings.Replace(checksum, ":", "\t", 1) {
		t.Errorf("preview CHECKSUM = %q, want %q", got, checksum)
	}
	files := dm01Lines(preview, "FILE")
	wantFiles := []string{
		"attendance-requests/" + f("1019.jpg"),
		"attendance/" + f("0915-in.jpg"), "attendance/" + f("1020-in.jpg"), "attendance/" + f("1020-out.jpg"),
		"leave-attachments/" + f("1001.jpg"),
		"loan-attachments/" + f("paid-col.pdf"), "loan-attachments/" + f("paid.pdf"),
		"overtime-attachments/" + f("1020.pdf"),
	}
	slices.Sort(files)
	slices.Sort(wantFiles)
	if !slices.Equal(files, wantFiles) {
		t.Errorf("preview FILE =\n%s\nwant\n%s", strings.Join(files, "\n"), strings.Join(wantFiles, "\n"))
	}
	if got := dm01Count(preview, "BERKAS", "overtime_requests.attachments\tdipakai_baris_tetap"); got != "1" {
		t.Errorf("preview BERKAS shared overtime file = %q, want 1", got)
	}

	h.dm01Core()

	dates := h.scalar(`SELECT string_agg(date::text, ',' ORDER BY date) FROM hris.attendance WHERE employee_id = $1`, e)
	if dates != "2026-10-21" {
		t.Errorf("attendance left = %s, want 2026-10-21", dates)
	}
	if got := h.scalar(`SELECT string_agg(id::text || '/' || COALESCE(attendance_id::text, 'null'), ',')
		FROM hris.attendance_requests WHERE employee_id = $1`, e); got != req1022+"/null" {
		t.Errorf("requests left = %s, want %s/null (attendance 20 Oct deleted → SET NULL)", got, req1022)
	}
	ids := func(table string) []string {
		got := strings.Split(h.scalar(`SELECT string_agg(id::text, ',' ORDER BY id) FROM hris.`+table+` WHERE employee_id = $1`, e), ",")
		slices.Sort(got)
		return got
	}
	sorted := func(s ...string) []string { slices.Sort(s); return s }
	if got := ids("leaves"); !slices.Equal(got, sorted(lv1822, lv1025)) {
		t.Errorf("leaves left = %v, want 18–22 and 25 Oct", got)
	}
	if got := ids("overtime_requests"); !slices.Equal(got, sorted(ot1021)) {
		t.Errorf("overtime left = %v, want 21 Oct", got)
	}
	if got := ids("loans"); !slices.Equal(got, sorted(lnRunning, lnLate)) {
		t.Errorf("loans left = %v, want running and 25 Oct", got)
	}
	if got := h.scalar(dm01Checksum); got != checksum {
		t.Errorf("leave_balances checksum %s, want %s", got, checksum)
	}
	if got := h.scalar(`SELECT concat_ws('/', annual_leave_total, annual_leave_used, sick_leave_used, updated_at)
		FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, e); got != balance {
		t.Errorf("leave balance %s, want %s", got, balance)
	}
	if got := notifications(); got != notesBefore {
		t.Errorf("notifications %s, want %s (no new rows)", got, notesBefore)
	}
	audit := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'data_cleanup.dm01'
		AND actor_name = 'Sistem (cleanup DM-01)' AND entity = 'hris' AND created_at = now()`)
	if audit != "1" {
		t.Fatalf("audit rows = %s, want 1", audit)
	}
	for _, table := range dm01Tables {
		got := h.scalar(`SELECT after->>$1 FROM audit.audit_log WHERE action = 'data_cleanup.dm01' AND created_at = now()`, table)
		if got != want[table] {
			t.Errorf("audit after.%s = %q, want %s", table, got, want[table])
		}
	}
	if got := h.scalar(`SELECT string_agg(tgname || '=' || tgenabled::text, ',' ORDER BY tgname) FROM pg_trigger
		WHERE tgname IN ('trg_notify_attendance_changes', 'trg_notify_leave_changes')`); got !=
		"trg_notify_attendance_changes=O,trg_notify_leave_changes=O" {
		t.Errorf("notify triggers = %s, want both enabled (O)", got)
	}
	// The closed-period guard is back for the rest of the transaction.
	h.exec("SAVEPOINT dm01_guard_after")
	if _, err := h.tx.Exec(context.Background(), `INSERT INTO hris.attendance (employee_id, date, status)
		VALUES ($1, '2026-09-16', 'present')`, e); err == nil {
		t.Errorf("closed-period insert after the cleanup succeeded, want the guard error")
	}
	h.exec("ROLLBACK TO SAVEPOINT dm01_guard_after")
	// The attendance trigger fires again, on its own condition only.
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, is_late, late_minutes)
		VALUES ($1, '2026-10-23', '2026-10-23 08:30+07', 'late', true, 30)`, e)
	if got := hrNotifications(); got != plusOne(t, hrBefore) {
		t.Errorf("HR notifications after a late clock-in = %s, want %s + 1", got, hrBefore)
	}
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-24', '2026-10-24 07:55+07', 'present')`, e)
	if got := hrNotifications(); got != plusOne(t, hrBefore) {
		t.Errorf("HR notifications after an on-time clock-in = %s, want %s + 1", got, hrBefore)
	}

	after := h.dm01Preview()
	for _, table := range dm01Tables {
		if got := dm01Count(after, "HAPUS", table); got != "0" {
			t.Errorf("preview after cleanup HAPUS %s = %q, want 0", table, got)
		}
	}
	if got := dm01Lines(after, "FILE"); len(got) != 0 {
		t.Errorf("preview after cleanup FILE = %v, want none", got)
	}
}

func plusOne(t *testing.T, n string) string {
	t.Helper()
	v, err := strconv.Atoi(n)
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(v + 1)
}

// TestDM01CleanupEdges: loan statuses outside the brief's list, the WIB date
// of a timestamptz request, business trips and the inclusive boundary.
func TestDM01CleanupEdges(t *testing.T) {
	h := newHarness(t)
	e := h.employee(emp{name: "DM01 Tepi"})

	loan := func(requested, status string, remaining *int, active bool) string {
		return h.scalar(`INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
			remaining_balance, status, requested_date, is_active)
			VALUES ($1, 'kasbon', 1000000, 2, 500000, $2, $3, $4::timestamptz, $5) RETURNING id::text`,
			e, remaining, status, requested, active)
	}
	zero, half := 0, 500000
	loan("2026-10-05 09:00+07", "paid_off", &zero, false)        // lunas lewat payroll → hapus
	loan("2026-10-05 09:00+07", "approved", nil, true)           // approved tanpa sisa → hapus
	loan("2026-10-05 09:00+07", "cancelled", &half, false)       // → hapus
	loan("2026-10-20 23:59+07", "pending", &half, true)          // 20 Okt WIB → hapus
	loan("2026-10-05 09:00+07", "manager_approved", &half, true) // menunggu HR = pending → hapus
	keptInactive := loan("2026-10-05 09:00+07", "approved", &half, false)
	keptWIB := loan("2026-10-21 01:00+07", "pending", &half, true) // 20 Okt 18:00 UTC, 21 Okt WIB
	keptNoDate := loan("2026-10-05 09:00+07", "pending", &half, true)
	h.exec(`UPDATE hris.loans SET requested_date = NULL WHERE id = $1`, keptNoDate)

	trip := func(start, end string) string {
		return h.scalar(`INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status)
			VALUES ($1, $2, $3, 'Bandung', 'Audit toko', 'approved') RETURNING id::text`, e, start, end)
	}
	trip("2026-10-05", "2026-10-06")
	keptTrip := trip("2026-10-19", "2026-10-22")
	h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
		VALUES ($1, 'sick', '2026-10-20', '2026-10-20', 1, 'Sakit uji DM-01', 'rejected')`, e)

	preview := h.dm01Preview()
	want := map[string]string{"attendance_requests": "0", "attendance": "0", "leaves": "1",
		"overtime_requests": "0", "business_trips": "1", "loans": "5"}
	for _, table := range dm01Tables {
		if got := dm01Count(preview, "HAPUS", table); got != want[table] {
			t.Errorf("preview HAPUS %s = %q, want %s\n%s", table, got, want[table], strings.Join(preview, "\n"))
		}
	}
	for key, n := range map[string]string{
		"loans\tberjalan (approved, sisa > 0)": "1",
		"loans\ttanpa requested_date":          "1",
		"business_trips\tmelintasi 21 Okt":     "1",
	} {
		if got := dm01Count(preview, "PENGECUALIAN", key); got != n {
			t.Errorf("preview PENGECUALIAN %s = %q, want %s", key, got, n)
		}
	}

	h.dm01Core()

	got := strings.Split(h.scalar(`SELECT string_agg(id::text, ',') FROM hris.loans WHERE employee_id = $1`, e), ",")
	slices.Sort(got)
	wantLoans := []string{keptInactive, keptWIB, keptNoDate}
	slices.Sort(wantLoans)
	if !slices.Equal(got, wantLoans) {
		t.Errorf("loans left = %v, want %v", got, wantLoans)
	}
	if got := h.scalar(`SELECT string_agg(id::text, ',') FROM hris.business_trips WHERE employee_id = $1`, e); got != keptTrip {
		t.Errorf("business trips left = %s, want the one crossing 21 Oct", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.leaves WHERE employee_id = $1`, e); got != "0" {
		t.Errorf("leaves left = %s, want 0 (end_date = 20 Oct is inclusive)", got)
	}
	// The core script's predicates match the preview's.
	for _, table := range dm01Tables {
		if got := h.scalar(`SELECT after->>$1 FROM audit.audit_log WHERE action = 'data_cleanup.dm01' AND created_at = now()`, table); got != want[table] {
			t.Errorf("audit after.%s = %q, want %s (preview)", table, got, want[table])
		}
	}
}

// TestDM01CoreNeedsTransaction: outside one transaction the cutoff set by
// SET LOCAL is gone and the script stops before touching anything.
func TestDM01CoreNeedsTransaction(t *testing.T) {
	core := dm01Script(t, "2026-11-03_dm01_cleanup_core.sql")
	for _, line := range strings.Split(core, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") && strings.Contains(line, "session_replication_role") {
			t.Errorf("core script must not use session_replication_role (R4): %s", line)
		}
	}
	for _, kw := range []string{"BEGIN;", "COMMIT;"} {
		if strings.Contains(core, "\n"+kw) {
			t.Errorf("core script contains %s; psql -1 owns the transaction", kw)
		}
	}
	h := newHarness(t)
	// What the guard sees when the SET LOCAL did not survive (autocommit).
	h.exec(`SELECT set_config('dm01.cutoff', '', true)`)
	start := strings.Index(core, "DO $$")
	end := strings.Index(core[start:], "END $$;")
	if start < 0 || end < 0 {
		t.Fatal("core script has no transaction guard (DO $$ … END $$;)")
	}
	h.exec("SAVEPOINT dm01_noTx")
	_, err := h.tx.Exec(context.Background(), core[start:start+end+len("END $$;")])
	h.exec("ROLLBACK TO SAVEPOINT dm01_noTx")
	if err == nil || !strings.Contains(err.Error(), "psql -1") {
		t.Fatalf("guard = %v, want an error naming psql -1", err)
	}
}

// TestDM01CoreRefusesChangedTriggers: ENABLE TRIGGER always sets 'O', so the
// script stops when a notify trigger is not enabled beforehand and leaves the
// state (and the data) as it found it.
func TestDM01CoreRefusesChangedTriggers(t *testing.T) {
	h := newHarness(t)
	e := h.employee(emp{name: "DM01 Trigger"})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, status) VALUES ($1, '2026-10-10', 'present')`, e)
	h.exec(`ALTER TABLE hris.leaves DISABLE TRIGGER trg_notify_leave_changes`)

	h.exec("SAVEPOINT dm01_triggers")
	_, err := h.tx.Exec(context.Background(), dm01Script(t, "2026-11-03_dm01_cleanup_core.sql"))
	h.exec("ROLLBACK TO SAVEPOINT dm01_triggers")
	if err == nil || !strings.Contains(err.Error(), "DM-01 dibatalkan") {
		t.Fatalf("core with a disabled trigger = %v, want the trigger guard error", err)
	}
	if got := h.scalar(`SELECT string_agg(tgname || '=' || tgenabled::text, ',' ORDER BY tgname) FROM pg_trigger
		WHERE tgname IN ('trg_notify_attendance_changes', 'trg_notify_leave_changes')`); got !=
		"trg_notify_attendance_changes=O,trg_notify_leave_changes=D" {
		t.Errorf("notify triggers = %s, want the state left unchanged (leave trigger D)", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.attendance WHERE employee_id = $1`, e); got != "1" {
		t.Errorf("attendance rows = %s, want 1 (nothing deleted)", got)
	}
}

// TestDM01PreviewFileSafety: only <folder of the column>/<row's employee>/<file>
// is a FILE; another folder or another employee's folder is FILE_LAIN.
func TestDM01PreviewFileSafety(t *testing.T) {
	h := newHarness(t)
	e := h.employee(emp{name: "DM01 Berkas"})
	other := h.employee(emp{name: "DM01 Berkas Lain"})
	leave := func(start, file string) {
		h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, attachment_url)
			VALUES ($1, 'annual', $2, $2, 1, 'Cuti uji berkas', 'approved', $3)`, e, start, file)
	}
	foreignFolder := "employee-documents/" + other + "/ktp.pdf"
	foreignOwner := "leave-attachments/" + other + "/1002.jpg"
	own := "leave-attachments/" + e + "/1003.jpg"
	leave("2026-10-01", foreignFolder)
	leave("2026-10-02", foreignOwner)
	leave("2026-10-03", own)
	leave("2026-10-04", "/srv/storage/private/leave-attachments/"+e+"/abs.jpg")
	leave("2026-10-05", "leave-attachments/"+e+"/../"+other+"/x.jpg")

	preview := h.dm01Preview()
	if got := dm01Lines(preview, "FILE"); !slices.Equal(got, []string{own}) {
		t.Errorf("FILE = %v, want only %s", got, own)
	}
	lain := dm01Lines(preview, "FILE_LAIN")
	for _, v := range []string{foreignFolder, foreignOwner} {
		if !slices.Contains(lain, v) {
			t.Errorf("FILE_LAIN %v lacks %s", lain, v)
		}
	}
	if len(lain) != 4 {
		t.Errorf("FILE_LAIN = %v, want 4 values", lain)
	}
	if got := dm01Count(preview, "BERKAS", "leaves.attachment_url\tdi_luar_folder_baris"); got != "4" {
		t.Errorf("BERKAS di_luar_folder_baris = %q, want 4", got)
	}
}

// TestDM01SameCutoff: the preview and the core script use one cutoff, the
// same literal, and no other date literal outside comments.
func TestDM01SameCutoff(t *testing.T) {
	date := regexp.MustCompile(`'\d{4}-\d{2}-\d{2}'`)
	for _, name := range []string{"2026-11-03_dm01_preview.sql", "2026-11-03_dm01_cleanup_core.sql"} {
		var found []string
		for _, line := range strings.Split(dm01Script(t, name), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			found = append(found, date.FindAllString(line, -1)...)
		}
		if !slices.Equal(found, []string{"'2026-10-20'"}) {
			t.Errorf("%s date literals = %v, want exactly one '2026-10-20'", name, found)
		}
	}
}

// TestDM01CoreChecksExpectations: the core deletes exactly the approved counts
// or nothing. Without counts it stops unless dm01.expect_skip = 'on'; a count
// that differs from what the DELETEs removed aborts the whole transaction
// (nothing deleted, the notify triggers left enabled).
func TestDM01CoreChecksExpectations(t *testing.T) {
	h := newHarness(t)
	e := h.employee(emp{name: "DM01 Angka"})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, status) VALUES ($1, '2026-10-10', 'present')`, e)
	h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
		VALUES ($1, 'annual', '2026-10-12', '2026-10-12', 1, 'Cuti uji angka', 'approved')`, e)
	core := dm01Script(t, "2026-11-03_dm01_cleanup_core.sql")
	left := func() string {
		return h.scalar(`SELECT (SELECT count(*) FROM hris.attendance WHERE employee_id = $1)::text || '/' ||
			(SELECT count(*) FROM hris.leaves WHERE employee_id = $1)::text`, e)
	}
	try := func(name string, setup func()) error {
		h.exec("SAVEPOINT " + name)
		setup()
		_, err := h.tx.Exec(context.Background(), core)
		h.exec("ROLLBACK TO SAVEPOINT " + name)
		return err
	}

	err := try("dm01_noexpect", func() {})
	if err == nil || !strings.Contains(err.Error(), "angka yang disetujui") {
		t.Fatalf("core without counts = %v, want the missing-counts error", err)
	}
	preview := h.dm01Preview()
	err = try("dm01_mismatch", func() { h.dm01Expect(preview, map[string]int{"attendance": 1}) })
	if err == nil || !strings.Contains(err.Error(), "beda dari angka yang disetujui") ||
		!strings.Contains(err.Error(), "attendance") {
		t.Fatalf("core with a wrong count = %v, want the mismatch error naming attendance", err)
	}
	if got := left(); got != "1/1" {
		t.Errorf("rows after refused runs = %s, want 1/1 (nothing deleted)", got)
	}
	if got := h.scalar(`SELECT string_agg(tgenabled::text, ',' ORDER BY tgname) FROM pg_trigger
		WHERE tgname IN ('trg_notify_attendance_changes', 'trg_notify_leave_changes')`); got != "O,O" {
		t.Errorf("notify triggers after refused runs = %s, want O,O", got)
	}

	h.exec(`SELECT set_config('dm01.expect_skip', 'on', true)`)
	h.exec(core)
	if got := left(); got != "0/0" {
		t.Errorf("rows after core with dm01.expect_skip = %s, want 0/0", got)
	}
}
