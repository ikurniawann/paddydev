package hris

import (
	"testing"
)

const rulesURL = "/api/hris/auto-checkout-rules"

// EPIC-080 R1/R2: aturan checkout otomatis per jabatan dan lokasi.
type rulesFixture struct {
	pos, pos2    string
	tokoA, tokoB string
	delegate     string
}

func newRulesFixture(h *harness) rulesFixture {
	f := rulesFixture{}
	f.pos = h.scalar(`INSERT INTO hris.positions (title) VALUES ('Kasir Aturan Uji') RETURNING id::text`)
	f.pos2 = h.scalar(`INSERT INTO hris.positions (title) VALUES ('Host Aturan Uji') RETURNING id::text`)
	f.tokoA, f.tokoB = h.branch("Aturan Toko A"), h.branch("Aturan Toko B")
	return f
}

func (h *harness) ruleAudit(action, ruleID string) string {
	h.t.Helper()
	return h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.auto_checkout_rules'
		AND action = $1 AND entity_id = $2`, action, ruleID)
}

func TestAutoCheckoutRulesHR(t *testing.T) {
	h := newHarness(t)
	f := newRulesFixture(h)
	hr, _ := h.staff("hrd", []string{"hris.kepegawaian.shifts"}, false)

	// create: one for a store, one for all locations
	c := h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos, "branch_id": f.tokoA})
	expect(t, c, 201, "")
	id := c.data()["id"].(string)
	if c.data()["is_active"] != true || c.data()["position_title"] != "Kasir Aturan Uji" || c.data()["editable"] != true {
		t.Fatalf("created rule = %s", c.raw)
	}
	all := h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos2, "branch_id": nil})
	expect(t, all, 201, "")
	allID := all.data()["id"].(string)
	if all.data()["branch_id"] != nil || all.data()["branch_name"] != nil {
		t.Fatalf("all-location rule must have no branch: %s", all.raw)
	}
	if h.ruleAudit("attendance.auto_checkout_rule.create", id) != "1" {
		t.Fatal("create must leave one audit row")
	}

	// duplicate (same store, and same all-location) -> 409
	expect(t, h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos, "branch_id": f.tokoA}), 409,
		"Aturan untuk jabatan dan lokasi ini sudah ada")
	expect(t, h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos2}), 409,
		"Aturan untuk jabatan dan lokasi ini sudah ada")

	// invalid input
	expect(t, h.do(&hr, "POST", rulesURL, map[string]any{"position_id": "bukan-uuid"}), 400, "")
	expect(t, h.do(&hr, "POST", rulesURL, map[string]any{"position_id": "11111111-1111-4111-8111-111111111111"}), 400, "Jabatan tidak ditemukan")
	expect(t, h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos, "branch_id": "11111111-1111-4111-8111-111111111111"}), 400, "Toko tidak ditemukan")

	// list: HR sees both, all editable
	l := h.do(&hr, "GET", rulesURL, nil)
	expect(t, l, 200, "")
	seen := map[string]map[string]any{}
	for _, x := range l.list() {
		m := x.(map[string]any)
		seen[m["id"].(string)] = m
	}
	if seen[id] == nil || seen[allID] == nil || seen[id]["editable"] != true || seen[allID]["editable"] != true {
		t.Fatalf("HR list = %s", l.raw)
	}
	if seen[id]["branch_name"] != "Aturan Toko A" {
		t.Fatalf("branch_name = %v", seen[id]["branch_name"])
	}

	// toggle
	p := h.do(&hr, "PATCH", rulesURL+"/"+id, map[string]any{"is_active": false})
	expect(t, p, 200, "")
	if p.data()["is_active"] != false {
		t.Fatalf("patched = %s", p.raw)
	}
	if h.scalar(`SELECT is_active::text FROM hris.auto_checkout_rules WHERE id = $1`, id) != "false" {
		t.Fatal("is_active not stored")
	}
	if h.ruleAudit("attendance.auto_checkout_rule.update", id) != "1" {
		t.Fatal("update must leave one audit row")
	}
	if h.scalar(`SELECT before->>'is_active' || '/' || (after->>'is_active') FROM audit.audit_log
		WHERE entity_id = $1 AND action = 'attendance.auto_checkout_rule.update'`, id) != "true/false" {
		t.Fatal("update audit must carry before/after")
	}
	expect(t, h.do(&hr, "PATCH", rulesURL+"/"+id, map[string]any{}), 400, "")
	expect(t, h.do(&hr, "PATCH", rulesURL+"/11111111-1111-4111-8111-111111111111", map[string]any{"is_active": true}), 404, "Aturan tidak ditemukan")

	// delete
	expect(t, h.do(&hr, "DELETE", rulesURL+"/"+id, nil), 200, "")
	if h.scalar(`SELECT count(*)::text FROM hris.auto_checkout_rules WHERE id = $1`, id) != "0" {
		t.Fatal("rule not deleted")
	}
	if h.ruleAudit("attendance.auto_checkout_rule.delete", id) != "1" {
		t.Fatal("delete must leave one audit row")
	}
	expect(t, h.do(&hr, "DELETE", rulesURL+"/"+id, nil), 404, "Aturan tidak ditemukan")

	// outsiders
	nobody, _ := h.staff("pos", nil, true)
	expect(t, h.do(&nobody, "GET", rulesURL, nil), 403, "Insufficient permissions")
	expect(t, h.do(&nobody, "POST", rulesURL, map[string]any{"position_id": f.pos}), 403, "Insufficient permissions")
}

func TestAutoCheckoutRulesDelegate(t *testing.T) {
	h := newHarness(t)
	f := newRulesFixture(h)
	hr, _ := h.staff("hrd", []string{"hris.kepegawaian.shifts"}, false)
	d, dID := h.staff("pos", nil, true, emp{name: "Pengatur Aturan"})
	h.delegate(dID, false, f.tokoA)

	allRule := h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos2})
	expect(t, allRule, 201, "")
	allID := allRule.data()["id"].(string)
	otherRule := h.do(&hr, "POST", rulesURL, map[string]any{"position_id": f.pos, "branch_id": f.tokoB})
	expect(t, otherRule, 201, "")
	otherID := otherRule.data()["id"].(string)

	// own store: allowed
	c := h.do(&d, "POST", rulesURL, map[string]any{"position_id": f.pos, "branch_id": f.tokoA})
	expect(t, c, 201, "")
	ownID := c.data()["id"].(string)
	if h.ruleAudit("attendance.auto_checkout_rule.create", ownID) != "1" {
		t.Fatal("delegate create must leave one audit row")
	}
	// the actor is the delegate's account
	if h.scalar(`SELECT actor_id::text FROM audit.audit_log WHERE entity_id = $1`, ownID) != d.UserID {
		t.Fatal("audit actor must be the delegate account")
	}

	const forbidden = "Hanya untuk toko yang Anda atur"
	// another store / all locations: 403
	expect(t, h.do(&d, "POST", rulesURL, map[string]any{"position_id": f.pos2, "branch_id": f.tokoB}), 403, forbidden)
	expect(t, h.do(&d, "POST", rulesURL, map[string]any{"position_id": f.pos2}), 403, forbidden)
	expect(t, h.do(&d, "POST", rulesURL, map[string]any{"position_id": f.pos2, "branch_id": nil}), 403, forbidden)

	// all-location and other-store rules cannot be changed or deleted
	expect(t, h.do(&d, "PATCH", rulesURL+"/"+allID, map[string]any{"is_active": false}), 403, forbidden)
	expect(t, h.do(&d, "DELETE", rulesURL+"/"+allID, nil), 403, forbidden)
	expect(t, h.do(&d, "PATCH", rulesURL+"/"+otherID, map[string]any{"is_active": false}), 403, forbidden)
	expect(t, h.do(&d, "DELETE", rulesURL+"/"+otherID, nil), 403, forbidden)
	if h.scalar(`SELECT is_active::text FROM hris.auto_checkout_rules WHERE id = $1`, allID) != "true" {
		t.Fatal("a refused PATCH must not change the rule")
	}

	// list: own-store rule (editable) + all-location rule (read-only); the other store's rule is hidden
	l := h.do(&d, "GET", rulesURL, nil)
	expect(t, l, 200, "")
	seen := map[string]map[string]any{}
	for _, x := range l.list() {
		m := x.(map[string]any)
		seen[m["id"].(string)] = m
	}
	if seen[otherID] != nil {
		t.Fatal("a rule of another store must not be listed")
	}
	if seen[allID] == nil || seen[allID]["editable"] != false {
		t.Fatalf("all-location rule must be listed read-only: %s", l.raw)
	}
	if seen[ownID] == nil || seen[ownID]["editable"] != true {
		t.Fatalf("own-store rule must be editable: %s", l.raw)
	}

	// toggle and delete the own rule
	expect(t, h.do(&d, "PATCH", rulesURL+"/"+ownID, map[string]any{"is_active": false}), 200, "")
	if h.ruleAudit("attendance.auto_checkout_rule.update", ownID) != "1" {
		t.Fatal("delegate update must leave one audit row")
	}
	expect(t, h.do(&d, "DELETE", rulesURL+"/"+ownID, nil), 200, "")
	if h.ruleAudit("attendance.auto_checkout_rule.delete", ownID) != "1" {
		t.Fatal("delegate delete must leave one audit row")
	}

	// options: delegate gets only their stores; HR gets every active store
	o := h.do(&d, "GET", rulesURL+"/options", nil)
	expect(t, o, 200, "")
	br := o.data()["branches"].([]any)
	if len(br) != 1 || br[0].(map[string]any)["id"] != f.tokoA {
		t.Fatalf("delegate options branches = %s", o.raw)
	}
	if o.data()["all_locations"] != false {
		t.Fatalf("delegate cannot pick all locations: %s", o.raw)
	}
	oh := h.do(&hr, "GET", rulesURL+"/options", nil)
	expect(t, oh, 200, "")
	if oh.data()["all_locations"] != true || len(oh.data()["positions"].([]any)) < 2 {
		t.Fatalf("HR options = %s", oh.raw)
	}
}
