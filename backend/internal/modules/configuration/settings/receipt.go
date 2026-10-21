package settings

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/configuration/settings/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/stalllabel"
)

// receiptStall is one scope option for the UI (configuration.warehouses).
//
// Every branch owns a "Main Storage", so Label is "<name> — <branch name>"
// when the list mixes branches (internal/platform/stalllabel).
type receiptStall struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BranchID   string `json:"branch_id"`
	BranchName string `json:"branch_name"`
	Label      string `json:"label"`
}

type receiptList struct {
	Success bool                     `json:"success"`
	Data    []domain.ReceiptSettings `json:"data"`
	Stalls  []receiptStall           `json:"stalls"`
}

func (h *handler) receiptSettings(r *http.Request) ([]domain.ReceiptSettings, error) {
	rows, err := h.receipts.ActiveRows(r.Context(), h.db)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ReceiptSettings, len(rows))
	for i, row := range rows {
		out[i] = domain.NormalizeReceipt(row)
	}
	return out, nil
}

func (h *handler) getReceipt(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.d.Auth.RequireMenuPrefix(r, iam.SettingsBusiness...); err != nil {
		return err
	}
	data, err := h.receiptSettings(r)
	if err != nil {
		return err
	}
	rows, err := h.db.Query(r.Context(),
		`SELECT w.id::text, w.name, w.branch_id::text, COALESCE(b.name, ''), w.is_default
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		ORDER BY w.name ASC`)
	if err != nil {
		return err
	}
	type stallRow struct {
		receiptStall
		isDefault bool
	}
	scanned, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (stallRow, error) {
		var s stallRow
		err := r.Scan(&s.ID, &s.Name, &s.BranchID, &s.BranchName, &s.isDefault)
		return s, err
	})
	if err != nil {
		return err
	}
	scanned = stalllabel.SortByBranch(scanned, func(s stallRow) stalllabel.Key {
		return stalllabel.Key{BranchName: s.BranchName, BranchID: s.BranchID, IsDefault: s.isDefault}
	}, nil)
	withBranch := stalllabel.Spans(scanned, func(s stallRow) string { return s.BranchID })
	stalls := make([]receiptStall, len(scanned))
	for i, s := range scanned {
		s.Label = stalllabel.Label(s.Name, s.BranchName, withBranch)
		stalls[i] = s.receiptStall
	}
	return httpx.JSON(w, http.StatusOK, receiptList{Success: true, Data: data, Stalls: orEmpty(stalls)})
}

func (h *handler) putReceipt(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.d.Auth.RequireMenuPrefix(r, iam.SettingsBusiness...); err != nil {
		return err
	}
	body, err := bodyObject(r)
	if err != nil {
		return err
	}
	in, err := domain.ParseReceiptScope(body)
	if err != nil {
		return httpx.BadRequest(err.Error())
	}
	// upsertReceiptScope: update the scope's active row, else insert one
	// (the global row under its fixed id).
	rows, err := h.receipts.ActiveRows(r.Context(), h.db)
	if err != nil {
		return err
	}
	now := h.d.Now()
	if existing := domain.FindReceiptScopeRow(rows, in); existing != nil {
		err = h.receipts.Update(r.Context(), h.db, *existing.ID, in, now)
	} else {
		var id *string
		if in.IsGlobal() {
			g := domain.ReceiptGlobalID
			id = &g
		}
		err = h.receipts.Insert(r.Context(), h.db, id, in, now)
	}
	if err != nil {
		return err
	}
	data, err := h.receiptSettings(r)
	if err != nil {
		return err
	}
	return httpx.Data(w, http.StatusOK, data)
}
