package hris

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// Persetujuan versi bagan (EPIC-102 R5/R6/R9). Hanya hris.org_chart_versions
// yang berubah: versi draft menjadi approved, versi approved sebelumnya
// menjadi archived, dalam satu transaksi bersama baris audit.

const (
	errOrgVersionNotFound = "Versi bagan tidak ditemukan"
	errOrgNotDraft        = "Versi ini sudah tidak berstatus draft"
	errOrgValidFrom       = "Tanggal berlaku tidak valid"
	errOrgRaceLost        = "Versi lain baru saja disetujui — muat ulang lalu coba lagi"
)

// POST /api/hris/org-chart/versions/{id}/approve { valid_from?, reason? }
func (h handlers) orgChartApprove(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		return httpx.NotFound(errOrgVersionNotFound)
	}
	body := optionalForm(r).Fields()
	validFrom := domain.TodayWIB(h.svc.now())
	if raw, sent := body["valid_from"]; sent && raw != nil {
		s, isStr := raw.(string)
		if _, okDate := domain.ParseDate(strings.TrimSpace(s)); !isStr || !okDate {
			return httpx.BadRequest(errOrgValidFrom)
		}
		validFrom = strings.TrimSpace(s)
	}
	var reason *string
	if raw, sent := body["reason"]; sent && raw != nil {
		s, isStr := raw.(string)
		if !isStr {
			return httpx.BadRequest("Alasan tidak valid")
		}
		reason = &s
	}

	ctx := r.Context()
	var approved orgVersion
	err := h.svc.repo.InTx(ctx, func(repo Repository) error {
		var err error
		approved, err = approveOrgVersion(ctx, repo.querier(), r, u, id, validFrom, reason)
		return err
	})
	if err != nil {
		return err
	}
	return ok(w, dataOf(approved))
}

// approveOrgVersion approves one draft on q (EPIC-102 R5): archive the
// approved one, approve this one, write the audit row.
func approveOrgVersion(ctx context.Context, q database.Querier, r *http.Request, u *auth.User, id, validFrom string, reason *string) (orgVersion, error) {
	// The row lock serialises two approvals of the same version.
	var label, status string
	err := q.QueryRow(ctx, `SELECT label, status FROM hris.org_chart_versions WHERE id = $1 FOR UPDATE`, id).Scan(&label, &status)
	if database.IsNoRows(err) {
		return orgVersion{}, httpx.NotFound(errOrgVersionNotFound)
	}
	if err != nil {
		return orgVersion{}, err
	}
	if status != "draft" {
		return orgVersion{}, httpx.Conflict(errOrgNotDraft)
	}
	nodes, err := orgNodes(ctx, q, id)
	if err != nil {
		return orgVersion{}, err
	}
	if errs, _ := domain.ValidateOrgChart(domainNodes(nodes)); len(errs) > 0 {
		return orgVersion{}, httpx.BadRequest(fmt.Sprintf("Bagan masih punya %d kesalahan struktur — perbaiki dulu sebelum disetujui", len(errs)), errs)
	}
	var archived *string
	rows, err := q.Query(ctx, `UPDATE hris.org_chart_versions SET status = 'archived'
		WHERE status = 'approved' AND id <> $1 RETURNING label`, id)
	if err != nil {
		return orgVersion{}, err
	}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			rows.Close()
			return orgVersion{}, err
		}
		archived = &l
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return orgVersion{}, err
	}
	tag, err := q.Exec(ctx, `UPDATE hris.org_chart_versions
		SET status = 'approved', valid_from = $2::date, approved_at = now(), approved_by_name = $3
		WHERE id = $1 AND status = 'draft'`, id, validFrom, u.FullName)
	if database.IsUniqueViolation(err) {
		// Another transaction approved a version first (EPIC-104 R2).
		return orgVersion{}, httpx.Conflict(errOrgRaceLost)
	}
	if err != nil {
		return orgVersion{}, err
	}
	if tag.RowsAffected() == 0 {
		return orgVersion{}, httpx.Conflict(errOrgNotDraft)
	}
	name := u.FullName
	if err := audit.Write(ctx, q, audit.Entry{
		ActorID: u.ID, ActorName: &name,
		Action: "hris.org_chart.version.approve", Entity: "hris.org_chart_versions",
		EntityID: &id, EntityLabel: &label,
		Before: map[string]any{"status": status},
		After:  map[string]any{"status": "approved", "valid_from": validFrom, "archived": archived},
		Reason: reason,
	}.WithRequest(r)); err != nil {
		return orgVersion{}, err
	}
	vs, err := orgVersions(ctx, q)
	if err != nil {
		return orgVersion{}, err
	}
	for _, v := range vs {
		if v.ID == id {
			return v, nil
		}
	}
	return orgVersion{}, httpx.NotFound(errOrgVersionNotFound)
}
