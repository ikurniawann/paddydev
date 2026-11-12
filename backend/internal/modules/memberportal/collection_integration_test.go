package memberportal

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/memberportal/domain"
	"nuhabit/backend/internal/platform/testutil"
)

// Integration tests for badges, rewards, collectibles and wallpapers. They
// skip without TEST_DATABASE_URL. WhatsApp and the XP ledger are faked so a
// test never sends a message or posts XP.

type collNotifier struct {
	sent chan string
}

func (n *collNotifier) SendOTP(context.Context, string, string, string) Delivery { return Delivery{} }

func (n *collNotifier) SendText(_ context.Context, target, message, _ string) Delivery {
	n.sent <- target + "|" + message
	return Delivery{Delivered: true}
}

type collLoyalty struct {
	mu     sync.Mutex
	awards []XPAward
}

func (l *collLoyalty) AwardFlatXP(_ context.Context, a XPAward) (XPResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.awards = append(l.awards, a)
	return XPResult{Status: "posted", XPAwarded: a.XPAmount}, nil
}

func (l *collLoyalty) AwardTopupXP(context.Context, string, float64, string) (XPResult, error) {
	return XPResult{Status: "skipped"}, nil
}

type collEnv struct {
	mux      http.Handler
	notifier *collNotifier
	loyalty  *collLoyalty
	svc      *Service
}

// pinClock fixes the service clock so time-window assertions do not depend
// on how long the requests took (EPIC-105 R6).
func (e collEnv) pinClock(at time.Time) { e.svc.now = func() time.Time { return at } }

func collSetup(t *testing.T) collEnv {
	t.Helper()
	m := New(testutil.Deps(t, nil), Options{})
	env := collEnv{notifier: &collNotifier{sent: make(chan string, 8)}, loyalty: &collLoyalty{}}
	m.handler.svc.notifier = env.notifier
	m.handler.svc.loyalty = env.loyalty
	env.svc = m.handler.svc
	env.mux = testutil.Mux(m)
	return env
}

func collID(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := testutil.DB(t).QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("fixture: %v\n%s", err, sql)
	}
	return id
}

func collExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := testutil.DB(t).Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec: %v\n%s", err, sql)
	}
}

func collCleanup(t *testing.T, sql string, args ...any) {
	t.Cleanup(func() { _, _ = testutil.DB(t).Exec(context.Background(), sql, args...) })
}

// collMember is a member with totalXP and, when profile is set, a CRM
// profile on the lowest tier.
func collMember(t *testing.T, totalXP int, profile bool) (testutil.Member, string) {
	t.Helper()
	m := testutil.CreateMember(t)
	collExec(t, `UPDATE pos.pos_customers SET total_xp = $2 WHERE id = $1`, m.CustomerID, totalXP)
	if !profile {
		return m, ""
	}
	profileID := collID(t, `INSERT INTO crm.crm_member_profiles (customer_id, tier_id)
		SELECT $1, id FROM crm.crm_membership_tiers ORDER BY rank LIMIT 1 RETURNING id::text`, m.CustomerID)
	return m, profileID
}

func collCode(kind string) string { return "go-test-" + kind + "-" + testutil.RandomHex(4) }

func collDo(t *testing.T, env collEnv, m *testutil.Member, method, path string, body any) (int, map[string]any, http.Header) {
	t.Helper()
	r := testutil.Request(method, prefix+path, body)
	if m != nil {
		r = testutil.AsMember(r, *m)
	}
	rec, out := testutil.Do(t, env.mux, r)
	return rec.Code, out, rec.Header()
}

func collExpect(t *testing.T, label string, status int, body map[string]any, wantStatus int, wantError string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("%s: status %d, want %d (%v)", label, status, wantStatus, body)
	}
	if wantError != "" && (body["success"] != false || body["error"] != wantError) {
		t.Fatalf("%s: body %v, want error %q", label, body, wantError)
	}
}

func collFind(t *testing.T, list any, id string) map[string]any {
	t.Helper()
	items, _ := list.([]any)
	for _, it := range items {
		if row, _ := it.(map[string]any); row["id"] == id {
			return row
		}
	}
	t.Fatalf("id %s not in %v", id, list)
	return nil
}

func TestCollectionRoutesNeedSession(t *testing.T) {
	env := collSetup(t)
	routes := []struct{ method, path string }{
		{"GET", "/badges"}, {"POST", "/badges"}, {"GET", "/rewards"}, {"POST", "/rewards"},
		{"GET", "/collectibles"}, {"POST", "/collectibles/equip"}, {"POST", "/collectibles/redeem"},
		{"GET", "/wallpapers"}, {"POST", "/wallpapers/redeem"},
	}
	for _, rt := range routes {
		status, body, _ := collDo(t, env, nil, rt.method, rt.path, nil)
		collExpect(t, rt.method+" "+rt.path, status, body, 401, "Unauthorized")
	}
}

func TestBadgesAwardAndShowcase(t *testing.T) {
	env := collSetup(t)
	member, _ := collMember(t, 500, true)

	earned := collID(t, `INSERT INTO crm.crm_badges (code, name, min_lifetime_xp, bonus_xp)
		VALUES ($1, 'Go Test Earned', 100, 50) RETURNING id::text`, collCode("badge"))
	collCleanup(t, `DELETE FROM crm.crm_badges WHERE id = $1`, earned)
	visits := collID(t, `INSERT INTO crm.crm_badges (code, name, min_lifetime_xp, metric, threshold)
		VALUES ($1, 'Go Test Visits', 0, 'visits', 1) RETURNING id::text`, collCode("badge"))
	collCleanup(t, `DELETE FROM crm.crm_badges WHERE id = $1`, visits)

	status, body, _ := collDo(t, env, &member, "GET", "/badges", nil)
	collExpect(t, "GET /badges", status, body, 200, "")
	data := body["data"].(map[string]any)
	if data["total_xp"] != float64(500) {
		t.Fatalf("total_xp = %v", data["total_xp"])
	}
	row := collFind(t, data["badges"], earned)
	if row["owned"] != true || row["awarded_at"] == nil || row["is_showcased"] != false ||
		row["metric"] != "lifetime_xp" || row["threshold"] != nil || row["min_lifetime_xp"] != float64(100) {
		t.Fatalf("earned badge row = %v", row)
	}
	if v := collFind(t, data["badges"], visits); v["owned"] != false || v["threshold"] != float64(1) || v["awarded_at"] != nil {
		t.Fatalf("visits badge without orders = %v", v)
	}

	env.loyalty.mu.Lock()
	var bonus *XPAward
	for i, a := range env.loyalty.awards {
		if a.SourceID == earned {
			bonus = &env.loyalty.awards[i]
		}
	}
	env.loyalty.mu.Unlock()
	if bonus == nil || bonus.XPAmount != 50 || bonus.SourceType != "badge_bonus" || bonus.ReferenceTable != "crm_badges" ||
		bonus.IdempotencyKey != "badge:"+earned+":"+member.CustomerID || bonus.Description != "Bonus badge: Go Test Earned" {
		t.Fatalf("bonus XP award = %+v", bonus)
	}
	select {
	case msg := <-env.notifier.sent:
		if !strings.HasPrefix(msg, member.Phone+"|Selamat! Kamu baru saja meraih badge: ") || !strings.Contains(msg, "Go Test Earned") {
			t.Fatalf("WhatsApp notice = %q", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no WhatsApp notice for the new badge")
	}

	// A second read awards nothing new.
	status, _, _ = collDo(t, env, &member, "GET", "/badges", nil)
	collExpect(t, "GET /badges again", status, nil, 200, "")
	select {
	case msg := <-env.notifier.sent:
		t.Fatalf("unexpected second notice %q", msg)
	case <-time.After(100 * time.Millisecond):
	}

	// Showcase: at most three.
	owned := []string{earned}
	for i := 0; i < 3; i++ {
		id := collID(t, `INSERT INTO crm.crm_badges (code, name, min_lifetime_xp, metric, is_active)
			VALUES ($1, 'Go Test Manual', 0, 'manual', false) RETURNING id::text`, collCode("badge"))
		collCleanup(t, `DELETE FROM crm.crm_badges WHERE id = $1`, id)
		collExec(t, `INSERT INTO crm.crm_member_badges (customer_id, badge_id, source) VALUES ($1, $2, 'manual')`, member.CustomerID, id)
		owned = append(owned, id)
	}
	for _, id := range owned[:3] {
		status, body, _ = collDo(t, env, &member, "POST", "/badges", map[string]any{"badge_id": id, "showcased": true})
		if status != 200 || body["success"] != true || len(body) != 1 {
			t.Fatalf("showcase %s: %d %v", id, status, body)
		}
	}
	status, body, _ = collDo(t, env, &member, "POST", "/badges", map[string]any{"badge_id": owned[3], "showcased": true})
	collExpect(t, "fourth showcase", status, body, 409, "Maksimal 3 badge dipamerkan — sembunyikan salah satu dulu.")
	status, body, _ = collDo(t, env, &member, "POST", "/badges", map[string]any{"badge_id": owned[0], "showcased": false})
	collExpect(t, "hide", status, body, 200, "")
	status, body, _ = collDo(t, env, &member, "POST", "/badges", map[string]any{"badge_id": visits, "showcased": false})
	collExpect(t, "not owned", status, body, 404, "Badge belum kamu miliki")
	status, body, _ = collDo(t, env, &member, "POST", "/badges", map[string]any{"badge_id": "nope"})
	collExpect(t, "bad id", status, body, 400, "Badge tidak valid")
	status, body, _ = collDo(t, env, &member, "POST", "/badges", "null")
	collExpect(t, "null body", status, body, 500, "Gagal menyimpan")
}

func TestRewardsCatalogAndRedeem(t *testing.T) {
	env := collSetup(t)
	member, _ := collMember(t, 500, true)

	reward := collID(t, `INSERT INTO crm.crm_rewards (code, name, reward_type, min_xp, max_redemptions_per_member, quota_period, stock_total)
		VALUES ($1, 'Go Test Voucher', 'voucher', 100, 1, 'monthly', 5) RETURNING id::text`, collCode("reward"))
	pricey := collID(t, `INSERT INTO crm.crm_rewards (code, name, reward_type, min_xp)
		VALUES ($1, 'Go Test Pricey', 'merchandise', 100000) RETURNING id::text`, collCode("reward"))
	collCleanup(t, `DELETE FROM crm.crm_rewards WHERE id = ANY($1::uuid[])`, []string{reward, pricey})
	collCleanup(t, `DELETE FROM crm.crm_redemptions WHERE reward_id = ANY($1::uuid[])`, []string{reward, pricey})

	status, body, _ := collDo(t, env, &member, "GET", "/rewards", nil)
	collExpect(t, "GET /rewards", status, body, 200, "")
	data := body["data"].(map[string]any)
	if m := data["member"].(map[string]any); m["total_xp"] != float64(500) || m["name"] != "Go Test Member" {
		t.Fatalf("member = %v", m)
	}
	row := collFind(t, data["rewards"], reward)
	if row["eligible"] != true || row["reason"] != nil || row["remaining_quota"] != float64(1) || row["remaining_stock"] != float64(5) ||
		row["quota_period_label"] != "Per bulan" || row["min_xp"] != float64(100) || row["xp_needed"] != float64(0) ||
		row["max_redemptions_per_member"] != float64(1) || row["required_tier_name"] != nil {
		t.Fatalf("reward row = %v", row)
	}
	if _, leaked := row["reward_data"]; leaked {
		t.Fatal("reward_data must not be returned")
	}
	if p := collFind(t, data["rewards"], pricey); p["eligible"] != false || p["reason"] != "XP kamu belum mencukupi" || p["xp_needed"] != float64(99500) {
		t.Fatalf("pricey row = %v", p)
	}

	status, body, _ = collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": reward})
	collExpect(t, "redeem", status, body, 200, "")
	red := body["data"].(map[string]any)
	if body["message"] != "Permintaan redeem terkirim. Tunjukkan kode ini ke kasir untuk pengambilan." ||
		red["status"] != "pending" || red["channel"] != "portal" || red["fulfilled_at"] != nil ||
		!strings.HasPrefix(red["redemption_number"].(string), "RDM-") || red["requested_at"] == nil {
		t.Fatalf("redeem body = %v", body)
	}
	var stock int
	if err := testutil.DB(t).QueryRow(context.Background(), `SELECT stock_redeemed FROM crm.crm_rewards WHERE id = $1`, reward).Scan(&stock); err != nil || stock != 1 {
		t.Fatalf("stock_redeemed = %d (%v)", stock, err)
	}

	status, body, _ = collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": reward})
	collExpect(t, "quota", status, body, 409, "Jatah redeem kamu untuk reward ini sudah habis")
	status, body, _ = collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": pricey})
	collExpect(t, "xp", status, body, 409, "XP kamu belum mencukupi")
	status, body, _ = collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": "00000000-0000-4000-8000-000000000000"})
	collExpect(t, "unknown", status, body, 404, "Reward tidak ditemukan")
	status, body, _ = collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": "x"})
	collExpect(t, "invalid", status, body, 400, "Reward tidak valid")
	status, body, _ = collDo(t, env, &member, "POST", "/rewards", "not json")
	collExpect(t, "not json", status, body, 500, "Gagal mengajukan redeem")

	status, body, _ = collDo(t, env, &member, "GET", "/rewards", nil)
	collExpect(t, "GET /rewards after", status, body, 200, "")
	data = body["data"].(map[string]any)
	if h := collFind(t, data["history"], red["id"].(string)); h["reward_name"] != "Go Test Voucher" || h["reward_type"] != "voucher" || h["status"] != "pending" {
		t.Fatalf("history row = %v", h)
	}
	if r := collFind(t, data["rewards"], reward); r["eligible"] != false || r["remaining_quota"] != float64(0) || r["remaining_stock"] != float64(4) {
		t.Fatalf("reward after redeem = %v", r)
	}
}

func TestRewardRedeemRateLimit(t *testing.T) {
	env := collSetup(t)
	env.pinClock(time.Now())
	member, _ := collMember(t, 0, false)
	for i := 0; i < domain.RedeemRateLimitCount; i++ {
		status, body, _ := collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": "x"})
		collExpect(t, "attempt", status, body, 400, "Reward tidak valid")
	}
	status, body, header := collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": "x"})
	collExpect(t, "11th attempt", status, body, 429, "Terlalu banyak percobaan. Coba lagi sebentar lagi.")
	if header.Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", header.Get("Retry-After"))
	}
}

func TestCollectiblesAvatars(t *testing.T) {
	env := collSetup(t)
	interval := domain.ParseIntervalXP(nil)
	if raw, err := newStore(testutil.DB(t)).CollectibleIntervalSetting(context.Background()); err == nil {
		interval = domain.ParseIntervalXP(raw)
	}
	member, profileID := collMember(t, interval, true)

	newAvatar := func(name string, stock *int, minXP int) string {
		id := collID(t, `INSERT INTO crm.crm_collectible_avatars (code, name, rarity, image_url, stock_total, min_lifetime_xp)
			VALUES ($1, $2, 'rare', 'https://example.test/a.png', $3, $4) RETURNING id::text`, collCode("avatar"), name, stock, minXP)
		collCleanup(t, `DELETE FROM crm.crm_collectible_avatars WHERE id = $1`, id)
		return id
	}
	zero := 0
	open := newAvatar("Go Test Open", nil, 0)
	owned := newAvatar("Go Test Owned", nil, 0)
	far := newAvatar("Go Test Far", nil, interval+1500)
	soldOut := newAvatar("Go Test Sold Out", &zero, 0)
	collExec(t, `INSERT INTO crm.crm_member_avatar_inventory (member_id, avatar_id, acquisition_source) VALUES ($1, $2, 'manual')`, profileID, owned)

	status, body, _ := collDo(t, env, &member, "POST", "/collectibles/equip", map[string]any{"avatar_id": owned})
	if status != 200 || body["success"] != true || body["message"] != "Artwork terpasang." {
		t.Fatalf("equip owned: %d %v", status, body)
	}
	var active *string
	if err := testutil.DB(t).QueryRow(context.Background(), `SELECT active_avatar_id::text FROM crm.crm_member_profiles WHERE id = $1`, profileID).Scan(&active); err != nil || active == nil || *active != owned {
		t.Fatalf("active_avatar_id = %v (%v)", active, err)
	}
	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/equip", map[string]any{"avatar_id": open})
	collExpect(t, "equip unowned", status, body, 403, "Artwork ini belum kamu miliki")
	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/equip", map[string]any{"avatar_id": "x"})
	collExpect(t, "equip invalid", status, body, 400, "Artwork tidak valid")

	status, body, _ = collDo(t, env, &member, "GET", "/collectibles", nil)
	collExpect(t, "GET /collectibles", status, body, 200, "")
	data := body["data"].(map[string]any)
	ent := data["entitlement"].(map[string]any)
	if ent["interval_xp"] != float64(interval) || ent["quota"] != float64(1) || ent["used"] != float64(0) || ent["remaining"] != float64(1) {
		t.Fatalf("entitlement = %v", ent)
	}
	if data["owned_count"] != float64(1) || data["member"].(map[string]any)["total_xp"] != float64(interval) {
		t.Fatalf("collectibles data = %v", data)
	}
	if first := data["items"].([]any)[0].(map[string]any); first["id"] != owned || first["equipped"] != true || first["acquired_at"] == nil || first["locked_reason"] != nil {
		t.Fatalf("owned item should sort first: %v", first)
	}
	if r := collFind(t, data["items"], far); r["locked_reason"] != "Kurang 1.500 XP lagi" || r["xp_needed"] != float64(1500) || r["rarity"] != "rare" {
		t.Fatalf("far item = %v", r)
	}
	if r := collFind(t, data["items"], open); r["locked_reason"] != "Belum kamu miliki" || r["remaining_stock"] != nil {
		t.Fatalf("open item = %v", r)
	}

	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/redeem", map[string]any{"avatar_id": owned})
	collExpect(t, "redeem owned", status, body, 409, "Artwork ini sudah kamu miliki")
	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/redeem", map[string]any{"avatar_id": soldOut})
	collExpect(t, "redeem sold out", status, body, 409, "Stok habis")
	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/redeem", map[string]any{"avatar_id": "00000000-0000-4000-8000-000000000000"})
	collExpect(t, "redeem unknown", status, body, 404, "Artwork tidak ditemukan")
	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/redeem", map[string]any{"avatar_id": 5})
	collExpect(t, "redeem invalid", status, body, 400, "Artwork tidak valid")

	status, body, _ = collDo(t, env, &member, "POST", "/collectibles/redeem", map[string]any{"avatar_id": open})
	var used int
	_ = testutil.DB(t).QueryRow(context.Background(), `SELECT count(*) FROM crm.crm_member_entitlements WHERE customer_id = $1`, member.CustomerID).Scan(&used)
	collExpect(t, "redeem open", status, body, 200, "")
	if body["message"] != "Artwork berhasil ditukar!" || used != 1 {
		t.Fatalf("redeem body = %v, used = %d", body, used)
	}
	if e := body["data"].(map[string]any)["entitlement"].(map[string]any); e["used"] != float64(1) || e["remaining"] != float64(0) {
		t.Fatalf("entitlement after = %v", e)
	}
}

func TestWallpapersRedeem(t *testing.T) {
	env := collSetup(t)
	interval := domain.ParseIntervalXP(nil)
	if raw, err := newStore(testutil.DB(t)).CollectibleIntervalSetting(context.Background()); err == nil {
		interval = domain.ParseIntervalXP(raw)
	}
	member, _ := collMember(t, interval, true)
	newWallpaper := func(name string) string {
		id := collID(t, `INSERT INTO crm.crm_collectible_wallpapers (code, name, rarity, image_url, stock_total)
			VALUES ($1, $2, 'epic', 'https://example.test/w.png', 3) RETURNING id::text`, collCode("wallpaper"), name)
		collCleanup(t, `DELETE FROM crm.crm_collectible_wallpapers WHERE id = $1`, id)
		return id
	}
	first := newWallpaper("Go Test Dawn")
	second := newWallpaper("Go Test Dusk")

	status, body, _ := collDo(t, env, &member, "GET", "/wallpapers", nil)
	collExpect(t, "GET /wallpapers", status, body, 200, "")
	data := body["data"].(map[string]any)
	if _, has := data["member"]; has {
		t.Fatal("wallpapers has no member block")
	}
	if r := collFind(t, data["items"], first); r["equipped"] != false || r["remaining_stock"] != float64(3) || r["owned"] != false {
		t.Fatalf("wallpaper item = %v", r)
	}

	status, body, _ = collDo(t, env, &member, "POST", "/wallpapers/redeem", map[string]any{"wallpaper_id": first})
	collExpect(t, "redeem", status, body, 200, "")
	if body["message"] != "Wallpaper berhasil ditukar!" {
		t.Fatalf("redeem body = %v", body)
	}
	if e := body["data"].(map[string]any)["entitlement"].(map[string]any); e["used"] != float64(1) || e["remaining"] != float64(0) || e["quota"] != float64(1) {
		t.Fatalf("entitlement after = %v", e)
	}
	status, body, _ = collDo(t, env, &member, "POST", "/wallpapers/redeem", map[string]any{"wallpaper_id": first})
	collExpect(t, "again", status, body, 409, "Wallpaper ini sudah kamu miliki")
	status, body, _ = collDo(t, env, &member, "POST", "/wallpapers/redeem", map[string]any{"wallpaper_id": second})
	collExpect(t, "quota used", status, body, 409, "Jatah tukar kamu sudah habis — kumpulkan XP lagi.")
	status, body, _ = collDo(t, env, &member, "POST", "/wallpapers/redeem", "{")
	collExpect(t, "broken json", status, body, 400, "Wallpaper tidak valid")

	status, body, _ = collDo(t, env, &member, "GET", "/wallpapers", nil)
	collExpect(t, "GET /wallpapers after", status, body, 200, "")
	data = body["data"].(map[string]any)
	if r := data["items"].([]any)[0].(map[string]any); r["id"] != first || r["owned"] != true || r["remaining_stock"] != float64(2) {
		t.Fatalf("owned wallpaper should sort first: %v", r)
	}

	noProfile, _ := collMember(t, interval, false)
	status, body, _ = collDo(t, env, &noProfile, "POST", "/wallpapers/redeem", map[string]any{"wallpaper_id": second})
	collExpect(t, "no profile", status, body, 409, "Profil loyalty belum aktif — lakukan satu transaksi ARK Coin dulu di kasir.")
	status, body, _ = collDo(t, env, &noProfile, "POST", "/collectibles/equip", map[string]any{"avatar_id": "00000000-0000-4000-8000-000000000000"})
	collExpect(t, "equip without profile", status, body, 409, "Profil member belum aktif")
}
