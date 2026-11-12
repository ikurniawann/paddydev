package memberportal

import (
	"testing"
	"time"

	"nuhabit/backend/internal/platform/testutil"
)

// Two harnesses are two API replicas on one database: the per-IP brake and
// the redeem brake must count hits from both.
func TestIntegrationIPBrakeIsSharedAcrossReplicas(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	b.ip = a.ip
	for i := range 30 {
		h := a
		if i%2 == 1 {
			h = b
		}
		if status, body, _ := h.do(testutil.Request("POST", "/api/member-portal/register/otp", map[string]any{"phone": "12"})); status != 400 {
			t.Fatalf("request %d: %d %v", i+1, status, body)
		}
	}
	status, body, _ := b.do(testutil.Request("POST", "/api/member-portal/otp", map[string]any{"phone": "12"}))
	if status != 429 || body["error"] != "Terlalu banyak permintaan dari jaringan ini. Coba lagi beberapa menit lagi" {
		t.Fatalf("31st request on the other replica: %d %v", status, body)
	}
	if status, _, _ := a.do(testutil.Request("POST", "/api/member-portal/verify", map[string]any{"phone": "0812", "code": "123456"})); status == 429 {
		t.Fatal("verify has its own per-IP budget")
	}
}

func TestIntegrationRedeemBrakeIsSharedAcrossReplicas(t *testing.T) {
	a, b := collSetup(t), collSetup(t)
	now := time.Now()
	a.pinClock(now)
	b.pinClock(now)
	member := newMember(t)
	for i := range 10 {
		env := a
		if i%2 == 1 {
			env = b
		}
		status, body, _ := collDo(t, env, &member, "POST", "/rewards", map[string]any{"reward_id": "x"})
		collExpect(t, "attempt", status, body, 400, "Reward tidak valid")
	}
	status, body, header := collDo(t, b, &member, "POST", "/rewards", map[string]any{"reward_id": "x"})
	collExpect(t, "11th attempt", status, body, 429, "Terlalu banyak percobaan. Coba lagi sebentar lagi.")
	if header.Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", header.Get("Retry-After"))
	}
}
