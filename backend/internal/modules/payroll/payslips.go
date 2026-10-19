package payroll

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Payslips (lib/payroll/payslips.ts). GET /api/hris/payslips/{id}/pdf stays
// in TS: it renders the PDF with a Node library.

// payslipRunEmbed is `payroll_run:payroll_runs ( … )`; the NULL employee
// column holds the employee embed's place before it.
const payslipRunEmbed = `NULL AS employee,
	(SELECT row_to_json(e) FROM (SELECT r.id, r.run_name, r.period_month, r.period_year, r.status, r.paid_at,
	        c.name AS company_name
	   FROM hris.payroll_runs r LEFT JOIN configuration.companies c ON c.id = r.company_id
	  WHERE r.id = d.payroll_run_id) e) AS payroll_run`

func (s *service) listPayslips(ctx context.Context, a *actor, employeeParam, runID *string, year, month *int) ([]*obj, error) {
	scope := domain.ResolvePayslipScope(a.Role, a.Payroll, a.EmployeeID, employeeParam)
	if scope.Skip {
		return []*obj{}, nil
	}
	where, args := []string{}, []any{}
	if scope.EmployeeID != "" {
		args = append(args, scope.EmployeeID)
		where = append(where, "d.employee_id = $"+itoa(len(args)))
	}
	if runID != nil {
		args = append(args, *runID)
		where = append(where, "d.payroll_run_id = $"+itoa(len(args)))
	}
	// The period lives on the run (payroll_details has no period columns).
	for _, period := range []struct {
		col string
		v   *int
	}{{"period_year", year}, {"period_month", month}} {
		if period.v != nil && *period.v != 0 {
			args = append(args, *period.v)
			where = append(where, "d.payroll_run_id IN (SELECT id FROM hris.payroll_runs WHERE "+period.col+" = $"+itoa(len(args))+")")
		}
	}
	sql := `SELECT d.*, ` + payslipRunEmbed + `, ` + detailAllocationsJSON("d") + ` AS allocations FROM hris.payroll_details d`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := queryObjs(ctx, s.db, sql+` ORDER BY d.created_at DESC LIMIT 60`, args...)
	if err != nil {
		return nil, opaque(err)
	}
	if err := s.stitch(ctx, s.db, rows, embed{"employee", "employee_id", payslipEmp}); err != nil {
		return nil, err
	}
	if !scope.MeView {
		return rows, nil
	}
	// The personal view only lists slips of paid runs.
	paid := []*obj{}
	for _, r := range rows {
		if run, ok := r.Get("payroll_run").(*obj); ok && run.Get("status") == "paid" {
			paid = append(paid, r)
		}
	}
	return paid, nil
}

type notifyData struct {
	WaLink      *string `json:"wa_link"`
	PayslipSent bool    `json:"payslip_sent"`
}

func (s *service) notifyPayslip(ctx context.Context, detailID string) (*messageData, error) {
	var employeeID, runStatus string
	var month *int
	var year int
	err := s.db.QueryRow(ctx, `SELECT d.employee_id::text, r.status, r.period_month, r.period_year
		  FROM hris.payroll_details d JOIN hris.payroll_runs r ON r.id = d.payroll_run_id
		 WHERE d.id = $1`, detailID).Scan(&employeeID, &runStatus, &month, &year)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Slip tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	if runStatus != "paid" {
		return nil, httpx.BadRequest("Notifikasi hanya untuk run yang sudah dibayar")
	}
	emp, err := s.brief(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	name, phone := "undefined", (*string)(nil)
	if emp != nil {
		name, phone = emp.FullName, emp.Phone
	}
	link := domain.BuildWaLink(phone, fmt.Sprintf("Halo %s, slip gaji Anda periode %s sudah terbit. "+
		"Silakan lihat detailnya di portal karyawan: menu Area Karyawan → Slip Gaji.", name, domain.PeriodLabelID(month, year)))
	if _, err := s.db.Exec(ctx, `UPDATE hris.payroll_details SET payslip_sent = true, payslip_sent_at = $2 WHERE id = $1`,
		detailID, s.clock()); err != nil {
		// The TS ignores this update's result.
		s.log.ErrorContext(ctx, "payslip: mark sent failed", "id", detailID, "error", err)
	}
	msg := "Slip ditandai terkirim (karyawan tidak punya nomor telepon utk WA)"
	if link != nil {
		msg = "Slip ditandai terkirim — buka WhatsApp untuk mengirim notifikasi"
	}
	return &messageData{Data: notifyData{link, true}, Message: msg}, nil
}

// GET /api/hris/payslips?employee_id&payroll_run_id&year&month
func (h *handler) listPayslips(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	q := queryForm(r)
	employee := q.Str("employee_id", optional, validate.StrOpts{})
	runID := q.Str("payroll_run_id", optional, validate.StrOpts{})
	year := coerceInt(q, "year", numRule{Rule: optional})
	month := coerceInt(q, "month", numRule{Rule: optional})
	if err := issuesErr(q, ""); err != nil {
		return err
	}
	rows, err := h.svc.listPayslips(r.Context(), a, employee, runID, year, month)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{rows})
}

// POST /api/hris/payslips/notify { payroll_detail_id }
func (h *handler) notifyPayslip(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	id := f.UUID("payroll_detail_id", validate.Rule{})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.notifyPayslip(r.Context(), *id)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}
