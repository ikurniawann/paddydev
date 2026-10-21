package settings_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/configuration/settings/domain"
	"nuhabit/backend/internal/platform/testutil"
)

func themeJSON(t *testing.T, a domain.Appearance) string {
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAppearance(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	var holdingID, companyID string
	if err := db.QueryRow(ctx, `INSERT INTO configuration.holdings (name, code) VALUES ('Go AP', $1) RETURNING id::text`,
		"GOAP"+testutil.RandomHex(3)).Scan(&holdingID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(ctx, `DELETE FROM configuration.holdings WHERE id = $1`, holdingID) })
	if err := db.QueryRow(ctx, `INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'Go AP Co', 'C') RETURNING id::text`,
		holdingID).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	level := "company"
	scoped := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.appearance": nil}, CompanyID: &companyID})
	if _, err := db.Exec(ctx, `UPDATE configuration.users SET business_scope = $2 WHERE id = $1`, scoped.UserID, level); err != nil {
		t.Fatal(err)
	}
	viewer := staff(t, "settings.roles")
	e := newEnv(t)

	def := themeJSON(t, domain.DefaultAppearance)
	expect(t, e.do(nil, "GET", "/api/settings/appearance", nil), 200,
		`{"data":{"company_id":null,"company_name":null,"companies":[],"theme":`+def+`}}`)

	company := `[{"id":"` + companyID + `","name":"Go AP Co"}]`
	expect(t, e.do(&scoped, "GET", "/api/settings/appearance", nil), 200,
		`{"data":{"company_id":"`+companyID+`","company_name":"Go AP Co","companies":`+company+`,"theme":`+def+`}}`)
	expect(t, e.do(&scoped, "GET", "/api/settings/appearance?company_id="+holdingID, nil), 403,
		`{"success":false,"error":"Company tidak dapat diakses"}`)

	expect(t, e.do(&viewer, "PUT", "/api/settings/appearance", map[string]any{}), 403, forbidden)
	expect(t, e.do(&scoped, "PUT", "/api/settings/appearance", "{bad"), 500, serverError)
	expect(t, e.do(&scoped, "PUT", "/api/settings/appearance", "null"), 400, `{"success":false,"error":"company_id wajib"}`)
	expect(t, e.do(&scoped, "PUT", "/api/settings/appearance", map[string]any{"company_id": holdingID}), 403,
		`{"success":false,"error":"Company tidak dapat diakses"}`)

	saved := domain.DefaultAppearance
	saved.PresetID = "ocean"
	saved.Base.Primary = "#0ea5e9"
	saved.Font = domain.AppearanceFont{Family: "inter", Size: 14}
	r := e.do(&scoped, "PUT", "/api/settings/appearance", map[string]any{"company_id": companyID,
		"theme": map[string]any{"presetId": "ocean", "base": map[string]any{"primary": "0EA5E9"}, "font": map[string]any{"family": "inter", "size": 14}}})
	expect(t, r, 200, `{"message":"Tema perusahaan tersimpan","data":{"company_id":"`+companyID+`","company_name":"Go AP Co","companies":`+
		company+`,"theme":`+themeJSON(t, saved)+`}}`)
	if got := e.text(`SELECT updated_by::text FROM configuration.company_appearance WHERE company_id = $1`, companyID); got != scoped.UserID {
		t.Fatalf("updated_by %s", got)
	}
	expect(t, e.do(&scoped, "GET", "/api/settings/appearance?company_id="+companyID, nil), 200,
		`{"data":{"company_id":"`+companyID+`","company_name":"Go AP Co","companies":`+company+`,"theme":`+themeJSON(t, saved)+`}}`)

	// A user without a business scope reaches every active company.
	unscoped := e.do(&viewer, "GET", "/api/settings/appearance?company_id="+companyID, nil)
	expect(t, unscoped, 200, "")
	if !strings.Contains(unscoped.Raw, `"company_id":"`+companyID+`","company_name":"Go AP Co"`) {
		t.Fatalf("unscoped %s", unscoped.Raw)
	}
}

func TestReceipt(t *testing.T) {
	admin := staff(t, "settings.business")
	e := newEnv(t)
	e.exec(`DELETE FROM pos.pos_receipt_settings`)
	branchID := e.text(`SELECT id::text FROM configuration.branches LIMIT 1`)
	whID := e.text(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'AAA Go Stall', $2) RETURNING id::text`,
		branchID, "GO-"+testutil.RandomHex(3))

	r := e.do(&admin, "GET", "/api/settings/receipt", nil)
	expect(t, r, 200, "")
	if !strings.HasPrefix(r.Raw, `{"success":true,"data":[],"stalls":[`) ||
		!strings.Contains(r.Raw, `{"id":"`+whID+`","name":"AAA Go Stall","branch_id":"`+branchID+`","branch_name":"`) {
		t.Fatalf("GET %s", r.Raw)
	}

	expect(t, e.do(&admin, "PUT", "/api/settings/receipt", map[string]any{"warehouse_id": whID}), 400,
		`{"success":false,"error":"Scope warehouse membutuhkan branch_id"}`)
	expect(t, e.do(&admin, "PUT", "/api/settings/receipt", "null"), 500, serverError)
	expect(t, e.do(&admin, "PUT", "/api/settings/receipt", map[string]any{"header_lines": []any{"  BCD Coffee ", "", 4, strings.Repeat("x", 50)},
		"footer_lines": "nope"}), 200,
		`{"success":true,"data":[{"id":"`+domain.ReceiptGlobalID+`","branch_id":null,"warehouse_id":null,"header_lines":["BCD Coffee","`+
			strings.Repeat("x", 42)+`"],"footer_lines":[],"show_stall_name":true,"updated_at":null}]}`)
	expect(t, e.do(&admin, "PUT", "/api/settings/receipt", map[string]any{"show_stall_name": false, "footer_lines": []string{"Terima kasih"}}), 200,
		`{"success":true,"data":[{"id":"`+domain.ReceiptGlobalID+`","branch_id":null,"warehouse_id":null,"header_lines":[],"footer_lines":["Terima kasih"],"show_stall_name":false,"updated_at":null}]}`)
	if got := e.text(`SELECT updated_at::text FROM pos.pos_receipt_settings WHERE id = $1`, domain.ReceiptGlobalID); !strings.HasPrefix(got, "2026-10-04 ") {
		t.Fatalf("updated_at %s", got)
	}

	r = e.do(&admin, "PUT", "/api/settings/receipt", map[string]any{"warehouse_id": whID, "branch_id": branchID, "header_lines": []string{"Stall"}})
	expect(t, r, 200, "")
	r = e.do(&admin, "PUT", "/api/settings/receipt", map[string]any{"warehouse_id": whID, "branch_id": branchID, "header_lines": []string{"Stall 2"}})
	if n := e.text(`SELECT count(*)::text FROM pos.pos_receipt_settings WHERE warehouse_id = $1`, whID); n != "1" ||
		!strings.Contains(r.Raw, `"warehouse_id":"`+whID+`","header_lines":["Stall 2"]`) {
		t.Fatalf("warehouse upsert %s %s", n, r.Raw)
	}
	expect(t, e.do(&admin, "PUT", "/api/settings/receipt", map[string]any{"warehouse_id": "nope", "branch_id": branchID}), 400,
		`{"success":false,"error":"Format data tidak valid"}`)
}

// The receipt scope select lists warehouses of every branch; each branch owns
// a "Main Storage", so the rows name their branch.
func TestReceiptStallLabels(t *testing.T) {
	admin := staff(t, "settings.business")
	e := newEnv(t)
	sfx := testutil.RandomHex(4)
	holding := e.text(`INSERT INTO configuration.holdings (name, code) VALUES ('H '||$1, 'H'||$1) RETURNING id::text`, sfx)
	company := e.text(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'C '||$2, 'C'||$2) RETURNING id::text`, holding, sfx)
	// Inserted Zeta first so the order cannot be insertion order.
	zeta := e.text(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Zeta '||$2, 'Z'||$2) RETURNING id::text`, company, sfx)
	alpha := e.text(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Alpha '||$2, 'A'||$2) RETURNING id::text`, company, sfx)
	stallZ := e.text(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Stall 1', 'STALL-01') RETURNING id::text`, zeta)
	mainA := e.text(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, alpha)
	mainZ := e.text(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, zeta)

	r := e.do(&admin, "GET", "/api/settings/receipt", nil)
	expect(t, r, 200, "")
	var body struct {
		Stalls []struct {
			ID         string `json:"id"`
			BranchID   string `json:"branch_id"`
			BranchName string `json:"branch_name"`
			Label      string `json:"label"`
		} `json:"stalls"`
	}
	if err := json.Unmarshal([]byte(r.Raw), &body); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, st := range body.Stalls {
		if st.BranchID == alpha || st.BranchID == zeta {
			got = append(got, st.ID+"|"+st.BranchName+"|"+st.Label)
		}
	}
	want := []string{
		mainA + "|Alpha " + sfx + "|Main Storage — Alpha " + sfx,
		mainZ + "|Zeta " + sfx + "|Main Storage — Zeta " + sfx,
		stallZ + "|Zeta " + sfx + "|Stall 1 — Zeta " + sfx,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stalls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
